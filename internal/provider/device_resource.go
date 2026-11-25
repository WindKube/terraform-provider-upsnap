package provider

import (
	"context"

	"github.com/WindKube/terraform-provider-upsnap/internal/client"
	"github.com/WindKube/terraform-provider-upsnap/internal/validators"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.ResourceWithConfigure        = &deviceResource{}
	_ resource.ResourceWithImportState      = &deviceResource{}
	_ resource.ResourceWithIdentity         = &deviceResource{}
	_ resource.ResourceWithConfigValidators = &deviceResource{}
)

func NewDeviceResource() resource.Resource {
	return &deviceResource{}
}

type deviceResource struct {
	client *client.Client
}

type deviceModel struct {
	ID                  types.String   `tfsdk:"id"`
	Name                types.String   `tfsdk:"name"`
	IP                  types.String   `tfsdk:"ip"`
	MAC                 types.String   `tfsdk:"mac"`
	Netmask             types.String   `tfsdk:"netmask"`
	Description         types.String   `tfsdk:"description"`
	Link                types.String   `tfsdk:"link"`
	LinkOpen            types.String   `tfsdk:"link_open"`
	TrackIP             types.Bool     `tfsdk:"track_ip"`
	GroupIDs            types.Set      `tfsdk:"group_ids"`
	PortIDs             types.Set      `tfsdk:"port_ids"`
	CreatedBy           types.String   `tfsdk:"created_by"`
	WOLPassword         types.String   `tfsdk:"wol_password"`
	WakeCmd             types.String   `tfsdk:"wake_cmd"`
	WakeConfirm         types.Bool     `tfsdk:"wake_confirm"`
	WakeTimeout         types.Int64    `tfsdk:"wake_timeout"`
	WakeCron            types.String   `tfsdk:"wake_cron"`
	WakeCronEnabled     types.Bool     `tfsdk:"wake_cron_enabled"`
	ShutdownCmd         types.String   `tfsdk:"shutdown_cmd"`
	ShutdownConfirm     types.Bool     `tfsdk:"shutdown_confirm"`
	ShutdownTimeout     types.Int64    `tfsdk:"shutdown_timeout"`
	ShutdownCron        types.String   `tfsdk:"shutdown_cron"`
	ShutdownCronEnabled types.Bool     `tfsdk:"shutdown_cron_enabled"`
	PingCmd             types.String   `tfsdk:"ping_cmd"`
	SOLEnabled          types.Bool     `tfsdk:"sol_enabled"`
	SOLPort             types.Int64    `tfsdk:"sol_port"`
	SOLAuth             types.Bool     `tfsdk:"sol_auth"`
	SOLUser             types.String   `tfsdk:"sol_user"`
	SOLPassword         types.String   `tfsdk:"sol_password"`
	Timeouts            timeouts.Value `tfsdk:"timeouts"`
}

func (r *deviceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_device"
}

func optionalString(description string, validators ...validator.String) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: description,
		Optional:            true,
		Computed:            true,
		Default:             stringdefault.StaticString(""),
		Validators:          validators,
	}
}

func optionalBool(description string) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: description,
		Optional:            true,
		Computed:            true,
		Default:             booldefault.StaticBool(false),
	}
}

func (r *deviceResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	secret := optionalString("Shutdown-on-LAN password (`sol_password`).")
	secret.Sensitive = true
	wolPassword := optionalString("SecureOn password sent with the magic packet (UpSnap field `password`). Must be empty or 4 or 6 characters long.",
		stringvalidator.Any(stringvalidator.LengthBetween(0, 0), stringvalidator.LengthBetween(4, 4), stringvalidator.LengthBetween(6, 6)))
	wolPassword.Sensitive = true

	resp.Schema = schema.Schema{
		MarkdownDescription: "A device UpSnap can wake, shut down and monitor (`devices` collection).\n\n" +
			"Ports are managed with `upsnap_port` and groups with `upsnap_group`. " +
			"When `created_by` names a user, UpSnap grants that user read, update, delete and power rights on the device, " +
			"which shows up as drift on an `upsnap_permission` that manages the same user.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Record ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name. UpSnap does not require names to be unique.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"ip": schema.StringAttribute{
				MarkdownDescription: "IPv4 address or fully qualified host name used for pings and wake/shutdown commands.",
				Required:            true,
				Validators:          []validator.String{validators.Host()},
			},
			"mac": schema.StringAttribute{
				MarkdownDescription: "MAC address the magic packet is sent to, for example `aa:bb:cc:dd:ee:ff`.",
				Required:            true,
				Validators:          []validator.String{validators.MAC()},
			},
			"netmask": schema.StringAttribute{
				MarkdownDescription: "Subnet mask used to compute the broadcast address. Defaults to `255.255.255.255`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("255.255.255.255"),
				Validators:          []validator.String{validators.Netmask()},
			},
			"description": optionalString("Free-form description."),
			"link":        optionalString("URL opened when the device card is clicked."),
			"link_open": optionalString("Where `link` opens: `same_tab`, `new_tab`, or empty for the UpSnap default.",
				stringvalidator.OneOf("", "same_tab", "new_tab")),
			"track_ip": optionalBool("Follow the device's IP address when it changes, found by MAC during network scans."),
			"group_ids": schema.SetAttribute{
				MarkdownDescription: "IDs of `upsnap_group`s the device belongs to.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				Default:             setdefault.StaticValue(types.SetValueMust(types.StringType, nil)),
				Validators:          []validator.Set{setvalidator.ValueStringsAre(validators.RecordID())},
			},
			"port_ids": schema.SetAttribute{
				MarkdownDescription: "IDs of the ports linked to the device. Manage ports with `upsnap_port`.",
				ElementType:         types.StringType,
				Computed:            true,
				PlanModifiers:       []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			},
			"created_by": schema.StringAttribute{
				MarkdownDescription: "ID of the UpSnap user that owns the device. Empty for devices created by a superuser.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Validators:          []validator.String{stringvalidator.Any(stringvalidator.LengthBetween(0, 0), validators.RecordID())},
			},
			"wol_password":          wolPassword,
			"wake_cmd":              optionalString("Shell command run instead of sending a magic packet. `{{ DEVICE_IP }}` and `{{ DEVICE_MAC }}` are substituted."),
			"wake_confirm":          optionalBool("Ask for confirmation in the UI before waking."),
			"wake_timeout":          optionalSeconds("Seconds UpSnap waits for the device to come online after a wake. UpSnap uses 120 when unset."),
			"wake_cron":             optionalString("Cron expression (five fields, or six with seconds) to wake the device on a schedule.", validators.Cron()),
			"wake_cron_enabled":     optionalBool("Enable `wake_cron`."),
			"shutdown_cmd":          optionalString("Shell command UpSnap runs to shut the device down, for example over SSH."),
			"shutdown_confirm":      optionalBool("Ask for confirmation in the UI before shutting down."),
			"shutdown_timeout":      optionalSeconds("Seconds UpSnap waits for the device to go offline after a shutdown. UpSnap uses 120 when unset."),
			"shutdown_cron":         optionalString("Cron expression (five fields, or six with seconds) to shut the device down on a schedule.", validators.Cron()),
			"shutdown_cron_enabled": optionalBool("Enable `shutdown_cron`."),
			"ping_cmd":              optionalString("Shell command used instead of ICMP ping to check whether the device is online."),
			"sol_enabled":           optionalBool("Shut down with [Sleep-On-LAN](https://github.com/SR-G/sleep-on-lan) instead of `shutdown_cmd`."),
			"sol_port": schema.Int64Attribute{
				MarkdownDescription: "Sleep-On-LAN port.",
				Optional:            true,
				Validators:          []validator.Int64{int64validator.Between(1, 65535)},
			},
			"sol_auth":     optionalBool("Sleep-On-LAN requires basic authentication."),
			"sol_user":     optionalString("Sleep-On-LAN user."),
			"sol_password": secret,
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true, Read: true, Update: true, Delete: true,
			}),
		},
	}
}

func optionalSeconds(description string) schema.Int64Attribute {
	return schema.Int64Attribute{
		MarkdownDescription: description,
		Optional:            true,
		Validators:          []validator.Int64{int64validator.AtLeast(1)},
	}
}

func (r *deviceResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		requiredWhenTrue{flag: "wake_cron_enabled", required: "wake_cron"},
		requiredWhenTrue{flag: "shutdown_cron_enabled", required: "shutdown_cron"},
	}
}

func (r *deviceResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("Device record ID.")
}

func (r *deviceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (m *deviceModel) input(ctx context.Context, diags *diag.Diagnostics) client.DeviceInput {
	return client.DeviceInput{
		Name:                m.Name.ValueString(),
		IP:                  m.IP.ValueString(),
		MAC:                 m.MAC.ValueString(),
		Netmask:             m.Netmask.ValueString(),
		Description:         m.Description.ValueString(),
		Link:                m.Link.ValueString(),
		LinkOpen:            m.LinkOpen.ValueString(),
		TrackIP:             m.TrackIP.ValueBool(),
		Groups:              setStrings(ctx, m.GroupIDs, diags),
		CreatedBy:           m.CreatedBy.ValueString(),
		Password:            m.WOLPassword.ValueString(),
		WakeCmd:             m.WakeCmd.ValueString(),
		WakeConfirm:         m.WakeConfirm.ValueBool(),
		WakeTimeout:         m.WakeTimeout.ValueInt64(),
		WakeCron:            m.WakeCron.ValueString(),
		WakeCronEnabled:     m.WakeCronEnabled.ValueBool(),
		ShutdownCmd:         m.ShutdownCmd.ValueString(),
		ShutdownConfirm:     m.ShutdownConfirm.ValueBool(),
		ShutdownTimeout:     m.ShutdownTimeout.ValueInt64(),
		ShutdownCron:        m.ShutdownCron.ValueString(),
		ShutdownCronEnabled: m.ShutdownCronEnabled.ValueBool(),
		PingCmd:             m.PingCmd.ValueString(),
		SOLEnabled:          m.SOLEnabled.ValueBool(),
		SOLPort:             m.SOLPort.ValueInt64(),
		SOLAuth:             m.SOLAuth.ValueBool(),
		SOLUser:             m.SOLUser.ValueString(),
		SOLPassword:         m.SOLPassword.ValueString(),
	}
}

func (m *deviceModel) fill(ctx context.Context, d *client.Device, diags *diag.Diagnostics) {
	m.ID = types.StringValue(d.ID)
	m.Name = types.StringValue(d.Name)
	m.IP = types.StringValue(d.IP)
	m.MAC = types.StringValue(d.MAC)
	m.Netmask = types.StringValue(d.Netmask)
	m.Description = types.StringValue(d.Description)
	m.Link = types.StringValue(d.Link)
	m.LinkOpen = types.StringValue(d.LinkOpen)
	m.TrackIP = types.BoolValue(d.TrackIP)
	m.GroupIDs = stringSet(ctx, d.Groups, diags)
	m.PortIDs = stringSet(ctx, d.Ports, diags)
	m.CreatedBy = types.StringValue(d.CreatedBy)
	m.WOLPassword = types.StringValue(d.Password)
	m.WakeCmd = types.StringValue(d.WakeCmd)
	m.WakeConfirm = types.BoolValue(d.WakeConfirm)
	m.WakeTimeout = int64OrNull(d.WakeTimeout)
	m.WakeCron = types.StringValue(d.WakeCron)
	m.WakeCronEnabled = types.BoolValue(d.WakeCronEnabled)
	m.ShutdownCmd = types.StringValue(d.ShutdownCmd)
	m.ShutdownConfirm = types.BoolValue(d.ShutdownConfirm)
	m.ShutdownTimeout = int64OrNull(d.ShutdownTimeout)
	m.ShutdownCron = types.StringValue(d.ShutdownCron)
	m.ShutdownCronEnabled = types.BoolValue(d.ShutdownCronEnabled)
	m.PingCmd = types.StringValue(d.PingCmd)
	m.SOLEnabled = types.BoolValue(d.SOLEnabled)
	m.SOLPort = int64OrNull(d.SOLPort)
	m.SOLAuth = types.BoolValue(d.SOLAuth)
	m.SOLUser = types.StringValue(d.SOLUser)
	m.SOLPassword = types.StringValue(d.SOLPassword)
}

func (r *deviceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan deviceModel
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
	device, err := r.client.Devices.Create(ctx, input)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "create device", err)
		return
	}
	plan.fill(ctx, device, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	setIDIdentity(ctx, resp.Identity, device.ID, &resp.Diagnostics)
}

func (r *deviceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state deviceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := state.Timeouts.Read(ctx, defaultTimeout)
	resp.Diagnostics.Append(diags...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	device, err := r.client.Devices.Get(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "read device", err)
		return
	}
	state.fill(ctx, device, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	setIDIdentity(ctx, resp.Identity, device.ID, &resp.Diagnostics)
}

func (r *deviceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan deviceModel
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
	device, err := r.client.Devices.Update(ctx, plan.ID.ValueString(), input)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "update device", err)
		return
	}
	plan.fill(ctx, device, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	setIDIdentity(ctx, resp.Identity, device.ID, &resp.Diagnostics)
}

func (r *deviceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state deviceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := state.Timeouts.Delete(ctx, defaultTimeout)
	resp.Diagnostics.Append(diags...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := r.client.Devices.Delete(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		apiErrorDiag(&resp.Diagnostics, "delete device", err)
	}
}

func (r *deviceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp, "", nil)
}
