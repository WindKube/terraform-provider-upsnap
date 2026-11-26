package provider

import (
	"context"

	"github.com/WindKube/terraform-provider-upsnap/internal/client"
	"github.com/WindKube/terraform-provider-upsnap/internal/validators"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const settingsID = "settings"

var (
	_ resource.ResourceWithConfigure   = &settingsResource{}
	_ resource.ResourceWithImportState = &settingsResource{}
	_ resource.ResourceWithIdentity    = &settingsResource{}
)

func NewSettingsResource() resource.Resource {
	return &settingsResource{}
}

type settingsResource struct {
	client *client.Client
}

type settingsModel struct {
	ID              types.String   `tfsdk:"id"`
	Interval        types.String   `tfsdk:"interval"`
	ScanRange       types.String   `tfsdk:"scan_range"`
	LazyPing        types.Bool     `tfsdk:"lazy_ping"`
	TrackIPInterval types.String   `tfsdk:"track_ip_interval"`
	WebsiteTitle    types.String   `tfsdk:"website_title"`
	SetupCompleted  types.Bool     `tfsdk:"setup_completed"`
	Timeouts        timeouts.Value `tfsdk:"timeouts"`
}

func (r *settingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_settings"
}

func adoptedString(description string, validators ...validator.String) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: description,
		Optional:            true,
		Computed:            true,
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		Validators:          validators,
	}
}

func (r *settingsResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "UpSnap's global settings, stored in the `settings_private` and `settings_public` singleton records. Requires superuser credentials.\n\n" +
			"There is only one settings object, so creating this resource adopts it, unset arguments keep their current values, " +
			"and destroying it only removes it from the Terraform state. " +
			"UpSnap applies the `UPSNAP_INTERVAL`, `UPSNAP_SCAN_RANGE` and `UPSNAP_WEBSITE_TITLE` environment variables on every start, " +
			"overriding these values; avoid setting both. The favicon cannot be managed.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Always `settings`.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"interval": adoptedString("Cron expression for how often devices are pinged, for example `*/3 * * * * *` (every three seconds).",
				validators.Cron(), stringvalidator.LengthAtLeast(1)),
			"scan_range":        adoptedString("Network range in CIDR notation scanned for new devices, for example `192.168.1.0/24`. Empty disables scanning.", validators.CIDR()),
			"track_ip_interval": adoptedString("Cron expression for how often devices with `track_ip` are looked up by MAC. Empty disables IP tracking.", validators.Cron()),
			"website_title":     adoptedString("Title shown in the browser tab and the web app manifest. Empty uses `UpSnap`."),
			"lazy_ping": schema.BoolAttribute{
				MarkdownDescription: "Only ping devices while someone has UpSnap open.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"setup_completed": schema.BoolAttribute{
				MarkdownDescription: "Whether a superuser exists, which UpSnap tracks to show its setup wizard.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true, Read: true, Update: true,
			}),
		},
	}
}

func (r *settingsResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("Always `settings`.")
}

func (r *settingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *settingsResource) records(ctx context.Context, diags *diag.Diagnostics) (*client.SettingsPrivate, *client.SettingsPublic) {
	priv, err := r.client.SettingsPrivate.List(ctx, client.ListOptions{PerPage: 1})
	if err != nil {
		apiErrorDiag(diags, "read private settings", err)
		return nil, nil
	}
	pub, err := r.client.SettingsPublic.List(ctx, client.ListOptions{PerPage: 1})
	if err != nil {
		apiErrorDiag(diags, "read public settings", err)
		return nil, nil
	}
	if len(priv) == 0 || len(pub) == 0 {
		diags.AddError("UpSnap settings not found", "UpSnap creates its settings records on start; none were found. Check that the endpoint is an UpSnap instance.")
		return nil, nil
	}
	return &priv[0], &pub[0]
}

func (m *settingsModel) fill(priv *client.SettingsPrivate, pub *client.SettingsPublic) {
	m.ID = types.StringValue(settingsID)
	m.Interval = types.StringValue(priv.Interval)
	m.ScanRange = types.StringValue(priv.ScanRange)
	m.LazyPing = types.BoolValue(priv.LazyPing)
	m.TrackIPInterval = types.StringValue(priv.TrackIPInterval)
	m.WebsiteTitle = types.StringValue(pub.WebsiteTitle)
	m.SetupCompleted = types.BoolValue(pub.SetupCompleted)
}

// apply writes the known plan values; unknown ones are left as they are in UpSnap.
func (r *settingsResource) apply(ctx context.Context, plan *settingsModel, diags *diag.Diagnostics) {
	priv, pub := r.records(ctx, diags)
	if diags.HasError() {
		return
	}

	privPatch := map[string]any{}
	setIfKnown(privPatch, "interval", plan.Interval)
	setIfKnown(privPatch, "scan_range", plan.ScanRange)
	setIfKnown(privPatch, "track_ip_interval", plan.TrackIPInterval)
	if !plan.LazyPing.IsUnknown() && !plan.LazyPing.IsNull() {
		privPatch["lazy_ping"] = plan.LazyPing.ValueBool()
	}
	if len(privPatch) > 0 {
		updated, err := r.client.SettingsPrivate.Update(ctx, priv.ID, privPatch)
		if err != nil {
			apiErrorDiag(diags, "update private settings", err)
			return
		}
		priv = updated
	}

	pubPatch := map[string]any{}
	setIfKnown(pubPatch, "website_title", plan.WebsiteTitle)
	if len(pubPatch) > 0 {
		updated, err := r.client.SettingsPublic.Update(ctx, pub.ID, pubPatch)
		if err != nil {
			apiErrorDiag(diags, "update public settings", err)
			return
		}
		pub = updated
	}
	plan.fill(priv, pub)
}

func setIfKnown(patch map[string]any, key string, v types.String) {
	if !v.IsUnknown() && !v.IsNull() {
		patch[key] = v.ValueString()
	}
}

func (r *settingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan settingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := plan.Timeouts.Create(ctx, defaultTimeout)
	resp.Diagnostics.Append(diags...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	r.apply(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	setIDIdentity(ctx, resp.Identity, settingsID, &resp.Diagnostics)
}

func (r *settingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state settingsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := state.Timeouts.Read(ctx, defaultTimeout)
	resp.Diagnostics.Append(diags...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	priv, pub := r.records(ctx, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	state.fill(priv, pub)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	setIDIdentity(ctx, resp.Identity, settingsID, &resp.Diagnostics)
}

func (r *settingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan settingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := plan.Timeouts.Update(ctx, defaultTimeout)
	resp.Diagnostics.Append(diags...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	r.apply(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	setIDIdentity(ctx, resp.Identity, settingsID, &resp.Diagnostics)
}

// Delete only forgets the settings: UpSnap always has exactly one settings object.
func (r *settingsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *settingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if id == "" && req.Identity != nil {
		var identity idIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		id = identity.ID.ValueString()
	}
	if id != settingsID {
		resp.Diagnostics.AddError("Invalid import ID", `upsnap_settings is a singleton; import it with the ID "settings".`)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), settingsID)...)
	setIDIdentity(ctx, resp.Identity, settingsID, &resp.Diagnostics)
}
