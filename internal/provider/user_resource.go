package provider

import (
	"context"
	"regexp"

	"github.com/WindKube/terraform-provider-upsnap/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.ResourceWithConfigure   = &userResource{}
	_ resource.ResourceWithImportState = &userResource{}
	_ resource.ResourceWithIdentity    = &userResource{}
)

var (
	usernamePattern = regexp.MustCompile(`^[\w][\w.\-]*$`)
	emailPattern    = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
)

func NewUserResource() resource.Resource {
	return &userResource{}
}

type userResource struct {
	client *client.Client
}

type userModel struct {
	ID              types.String   `tfsdk:"id"`
	Username        types.String   `tfsdk:"username"`
	Email           types.String   `tfsdk:"email"`
	EmailVisibility types.Bool     `tfsdk:"email_visibility"`
	Verified        types.Bool     `tfsdk:"verified"`
	Avatar          types.Int64    `tfsdk:"avatar"`
	Password        types.String   `tfsdk:"password"`
	Timeouts        timeouts.Value `tfsdk:"timeouts"`
}

func (r *userResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *userResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	email := optionalString("Email address.", stringvalidator.RegexMatches(emailPattern, "must be an email address"))
	resp.Schema = schema.Schema{
		MarkdownDescription: "An UpSnap user account (`users` collection). Grant it access to devices with `upsnap_permission`. " +
			"Requires superuser credentials.\n\n" +
			"UpSnap cannot return a password, so `password` is only compared with the value in state: changes made outside Terraform are not detected, " +
			"and after an import the next apply sets the configured password.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Record ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "Unique (case-insensitive) login name: 3-150 letters, digits, `_`, `.` or `-`, starting with a letter, digit or `_`.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(3, 150),
					stringvalidator.RegexMatches(usernamePattern, "must start with a letter, digit or underscore and contain only letters, digits, underscores, dots and dashes"),
				},
			},
			"email":            email,
			"email_visibility": optionalBool("Show the email address to other users."),
			"verified":         optionalBool("Mark the account as verified."),
			"avatar": schema.Int64Attribute{
				MarkdownDescription: "Index of the built-in avatar, 0-9.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
				Validators:          []validator.Int64{int64validator.Between(0, 9)},
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Login password, at least 5 characters.",
				Required:            true,
				Sensitive:           true,
				Validators:          []validator.String{stringvalidator.LengthBetween(5, 71)},
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true, Read: true, Update: true, Delete: true,
			}),
		},
	}
}

func (r *userResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("User record ID.")
}

func (r *userResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (m *userModel) input(withPassword bool) client.UserInput {
	in := client.UserInput{
		Username:        m.Username.ValueString(),
		Email:           m.Email.ValueString(),
		EmailVisibility: m.EmailVisibility.ValueBool(),
		Verified:        m.Verified.ValueBool(),
		Avatar:          m.Avatar.ValueInt64(),
	}
	if withPassword {
		in.Password, in.PasswordConfirm = m.Password.ValueString(), m.Password.ValueString()
	}
	return in
}

func (m *userModel) fill(u *client.User) {
	m.ID = types.StringValue(u.ID)
	m.Username = types.StringValue(u.Username)
	m.Email = types.StringValue(u.Email)
	m.EmailVisibility = types.BoolValue(u.EmailVisibility)
	m.Verified = types.BoolValue(u.Verified)
	m.Avatar = types.Int64Value(u.Avatar)
}

func (r *userResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := plan.Timeouts.Create(ctx, defaultTimeout)
	resp.Diagnostics.Append(diags...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	user, err := r.client.Users.Create(ctx, plan.input(true))
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "create user", err)
		return
	}
	plan.fill(user)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	setIDIdentity(ctx, resp.Identity, user.ID, &resp.Diagnostics)
}

func (r *userResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := state.Timeouts.Read(ctx, defaultTimeout)
	resp.Diagnostics.Append(diags...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	user, err := r.client.Users.Get(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "read user", err)
		return
	}
	state.fill(user)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	setIDIdentity(ctx, resp.Identity, user.ID, &resp.Diagnostics)
}

func (r *userResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state userModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := plan.Timeouts.Update(ctx, defaultTimeout)
	resp.Diagnostics.Append(diags...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	user, err := r.client.Users.Update(ctx, plan.ID.ValueString(), plan.input(!plan.Password.Equal(state.Password)))
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "update user", err)
		return
	}
	plan.fill(user)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	setIDIdentity(ctx, resp.Identity, user.ID, &resp.Diagnostics)
}

func (r *userResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state userModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := state.Timeouts.Delete(ctx, defaultTimeout)
	resp.Diagnostics.Append(diags...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := r.client.Users.Delete(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		apiErrorDiag(&resp.Diagnostics, "delete user", err)
	}
}

func (r *userResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp, map[string]importLookup{"username": r.lookupUsername})
}

func (r *userResource) lookupUsername(ctx context.Context, username string) (string, error) {
	return lookupUserID(ctx, r.client, username)
}

func lookupUserID(ctx context.Context, c *client.Client, username string) (string, error) {
	user, err := c.Users.FindOne(ctx, client.Eq("username", username))
	if err != nil {
		return "", err
	}
	return user.ID, nil
}
