package provider

import (
	"context"

	"github.com/WindKube/terraform-provider-upsnap/internal/client"
	"github.com/WindKube/terraform-provider-upsnap/internal/validators"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.ResourceWithConfigure   = &permissionResource{}
	_ resource.ResourceWithImportState = &permissionResource{}
	_ resource.ResourceWithIdentity    = &permissionResource{}
)

func NewPermissionResource() resource.Resource {
	return &permissionResource{}
}

type permissionResource struct {
	client *client.Client
}

type permissionModel struct {
	ID       types.String   `tfsdk:"id"`
	UserID   types.String   `tfsdk:"user_id"`
	Create   types.Bool     `tfsdk:"create"`
	Read     types.Set      `tfsdk:"read"`
	Update   types.Set      `tfsdk:"update"`
	Delete   types.Set      `tfsdk:"delete"`
	Power    types.Set      `tfsdk:"power"`
	Timeouts timeouts.Value `tfsdk:"timeouts"`
}

func (r *permissionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_permission"
}

func deviceIDSet(description string) schema.SetAttribute {
	return schema.SetAttribute{
		MarkdownDescription: description,
		ElementType:         types.StringType,
		Optional:            true,
		Computed:            true,
		Default:             setdefault.StaticValue(types.SetValueMust(types.StringType, nil)),
		Validators:          []validator.Set{setvalidator.ValueStringsAre(validators.RecordID())},
	}
}

func (r *permissionResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "What an UpSnap user may do (`permissions` collection). Each user has at most one permission record. " +
			"Requires superuser credentials.\n\n" +
			"UpSnap adds a device to the `read`, `update`, `delete` and `power` sets of its creator when a user creates a device, " +
			"or when `upsnap_device.created_by` is set, and removes deleted devices from every set. Both show up as drift here.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Record ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"user_id": schema.StringAttribute{
				MarkdownDescription: "ID of the `upsnap_user` the permissions apply to.",
				Required:            true,
				Validators:          []validator.String{validators.RecordID()},
			},
			"create": optionalBool("Allow the user to create devices and groups and to scan the network."),
			"read":   deviceIDSet("IDs of devices the user can see."),
			"update": deviceIDSet("IDs of devices the user can edit."),
			"delete": deviceIDSet("IDs of devices the user can delete."),
			"power":  deviceIDSet("IDs of devices the user can wake, sleep, reboot and shut down."),
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true, Read: true, Update: true, Delete: true,
			}),
		},
	}
}

func (r *permissionResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("Permission record ID.")
}

func (r *permissionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (m *permissionModel) input(ctx context.Context, diags *diag.Diagnostics) client.PermissionInput {
	return client.PermissionInput{
		User:   m.UserID.ValueString(),
		Create: m.Create.ValueBool(),
		Read:   setStrings(ctx, m.Read, diags),
		Update: setStrings(ctx, m.Update, diags),
		Delete: setStrings(ctx, m.Delete, diags),
		Power:  setStrings(ctx, m.Power, diags),
	}
}

func (m *permissionModel) fill(ctx context.Context, p *client.Permission, diags *diag.Diagnostics) {
	m.ID = types.StringValue(p.ID)
	m.UserID = types.StringValue(p.User)
	m.Create = types.BoolValue(p.Create)
	m.Read = stringSet(ctx, p.Read, diags)
	m.Update = stringSet(ctx, p.Update, diags)
	m.Delete = stringSet(ctx, p.Delete, diags)
	m.Power = stringSet(ctx, p.Power, diags)
}

func (r *permissionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan permissionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := plan.Timeouts.Create(ctx, defaultTimeout)
	resp.Diagnostics.Append(diags...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	input := plan.input(ctx, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	perm, err := r.client.Permissions.Create(ctx, input)
	if err != nil {
		if existing, findErr := r.client.Permissions.FindOne(ctx, client.Eq("user", input.User)); findErr == nil {
			resp.Diagnostics.AddError("Permissions already exist for this user",
				"User "+input.User+" already has permission record "+existing.ID+". Import it instead, for example with an import block using id = \"user:"+input.User+"\".")
			return
		}
		apiErrorDiag(&resp.Diagnostics, "create permissions", err)
		return
	}
	plan.fill(ctx, perm, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	setIDIdentity(ctx, resp.Identity, perm.ID, &resp.Diagnostics)
}

func (r *permissionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state permissionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := state.Timeouts.Read(ctx, defaultTimeout)
	resp.Diagnostics.Append(diags...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	perm, err := r.client.Permissions.Get(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "read permissions", err)
		return
	}
	state.fill(ctx, perm, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	setIDIdentity(ctx, resp.Identity, perm.ID, &resp.Diagnostics)
}

func (r *permissionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan permissionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := plan.Timeouts.Update(ctx, defaultTimeout)
	resp.Diagnostics.Append(diags...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	input := plan.input(ctx, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	perm, err := r.client.Permissions.Update(ctx, plan.ID.ValueString(), input)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "update permissions", err)
		return
	}
	plan.fill(ctx, perm, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	setIDIdentity(ctx, resp.Identity, perm.ID, &resp.Diagnostics)
}

func (r *permissionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state permissionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := state.Timeouts.Delete(ctx, defaultTimeout)
	resp.Diagnostics.Append(diags...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := r.client.Permissions.Delete(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		apiErrorDiag(&resp.Diagnostics, "delete permissions", err)
	}
}

func (r *permissionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp, map[string]importLookup{
		"user":     r.lookupByUserID,
		"username": r.lookupByUsername,
	})
}

func (r *permissionResource) lookupByUserID(ctx context.Context, userID string) (string, error) {
	perm, err := r.client.Permissions.FindOne(ctx, client.Eq("user", userID))
	if err != nil {
		return "", err
	}
	return perm.ID, nil
}

func (r *permissionResource) lookupByUsername(ctx context.Context, username string) (string, error) {
	userID, err := lookupUserID(ctx, r.client, username)
	if err != nil {
		return "", err
	}
	return r.lookupByUserID(ctx, userID)
}
