package provider

import (
	"context"
	"errors"

	"github.com/WindKube/terraform-provider-upsnap/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSourceWithConfigure        = &deviceDataSource{}
	_ datasource.DataSourceWithConfigValidators = &deviceDataSource{}
	_ datasource.DataSourceWithConfigure        = &devicesDataSource{}
)

type deviceDataModel struct {
	ID                  types.String `tfsdk:"id"`
	Name                types.String `tfsdk:"name"`
	IP                  types.String `tfsdk:"ip"`
	MAC                 types.String `tfsdk:"mac"`
	Netmask             types.String `tfsdk:"netmask"`
	Description         types.String `tfsdk:"description"`
	Link                types.String `tfsdk:"link"`
	Status              types.String `tfsdk:"status"`
	GroupIDs            types.Set    `tfsdk:"group_ids"`
	PortIDs             types.Set    `tfsdk:"port_ids"`
	CreatedBy           types.String `tfsdk:"created_by"`
	WakeCronEnabled     types.Bool   `tfsdk:"wake_cron_enabled"`
	ShutdownCronEnabled types.Bool   `tfsdk:"shutdown_cron_enabled"`
}

func (m *deviceDataModel) fill(ctx context.Context, d *client.Device, diags *diag.Diagnostics) {
	m.ID = types.StringValue(d.ID)
	m.Name = types.StringValue(d.Name)
	m.IP = types.StringValue(d.IP)
	m.MAC = types.StringValue(d.MAC)
	m.Netmask = types.StringValue(d.Netmask)
	m.Description = types.StringValue(d.Description)
	m.Link = types.StringValue(d.Link)
	m.Status = types.StringValue(d.Status)
	m.GroupIDs = stringSet(ctx, d.Groups, diags)
	m.PortIDs = stringSet(ctx, d.Ports, diags)
	m.CreatedBy = types.StringValue(d.CreatedBy)
	m.WakeCronEnabled = types.BoolValue(d.WakeCronEnabled)
	m.ShutdownCronEnabled = types.BoolValue(d.ShutdownCronEnabled)
}

func deviceDataAttributes(lookup bool) map[string]schema.Attribute {
	attrs := map[string]schema.Attribute{
		"id":                    schema.StringAttribute{Computed: true, MarkdownDescription: "Record ID."},
		"name":                  schema.StringAttribute{Computed: true, MarkdownDescription: "Display name."},
		"ip":                    schema.StringAttribute{Computed: true, MarkdownDescription: "IPv4 address or host name."},
		"mac":                   schema.StringAttribute{Computed: true, MarkdownDescription: "MAC address."},
		"netmask":               schema.StringAttribute{Computed: true, MarkdownDescription: "Subnet mask."},
		"description":           schema.StringAttribute{Computed: true, MarkdownDescription: "Description."},
		"link":                  schema.StringAttribute{Computed: true, MarkdownDescription: "Link URL."},
		"status":                schema.StringAttribute{Computed: true, MarkdownDescription: "Last known power state: `online`, `offline`, `pending`, or empty before the first ping."},
		"group_ids":             schema.SetAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Group IDs."},
		"port_ids":              schema.SetAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Port IDs."},
		"created_by":            schema.StringAttribute{Computed: true, MarkdownDescription: "Owning user ID, empty for superuser-created devices."},
		"wake_cron_enabled":     schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether scheduled wake is enabled."},
		"shutdown_cron_enabled": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether scheduled shutdown is enabled."},
	}
	if lookup {
		attrs["id"] = schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Record ID. Exactly one of `id` and `name` must be set."}
		attrs["name"] = schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Device name. Fails when no device or more than one device has this name."}
	}
	return attrs
}

func NewDeviceDataSource() datasource.DataSource {
	return &deviceDataSource{}
}

type deviceDataSource struct {
	client *client.Client
}

func (d *deviceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_device"
}

func (d *deviceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a single device by ID or by its unique name. Secrets such as `sol_password` are not exposed.",
		Attributes:          deviceDataAttributes(true),
	}
}

func (d *deviceDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *deviceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *deviceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg deviceDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var device *client.Device
	var err error
	if !cfg.ID.IsNull() {
		device, err = d.client.Devices.Get(ctx, cfg.ID.ValueString())
	} else {
		device, err = d.client.Devices.FindOne(ctx, client.Eq("name", cfg.Name.ValueString()))
	}
	switch {
	case client.IsNotFound(err):
		resp.Diagnostics.AddError("Device not found", "No device matches the given id or name, or the credentials cannot read it.")
		return
	case errors.Is(err, client.ErrNotUnique):
		resp.Diagnostics.AddAttributeError(path.Root("name"), "Device name is not unique", "More than one device has this name; look it up by id instead.")
		return
	case err != nil:
		apiErrorDiag(&resp.Diagnostics, "read device", err)
		return
	}
	cfg.fill(ctx, device, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

func NewDevicesDataSource() datasource.DataSource {
	return &devicesDataSource{}
}

type devicesDataSource struct {
	client *client.Client
}

type devicesDataModel struct {
	Name    types.String      `tfsdk:"name"`
	GroupID types.String      `tfsdk:"group_id"`
	Status  types.String      `tfsdk:"status"`
	Filter  types.String      `tfsdk:"filter"`
	Devices []deviceDataModel `tfsdk:"devices"`
}

func (d *devicesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_devices"
}

func (d *devicesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "List devices, optionally filtered. All filters are combined with AND.",
		Attributes: map[string]schema.Attribute{
			"name":     schema.StringAttribute{Optional: true, MarkdownDescription: "Only devices with exactly this name."},
			"group_id": schema.StringAttribute{Optional: true, MarkdownDescription: "Only devices in this group."},
			"status": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Only devices in this state: `online`, `offline` or `pending`.",
				Validators:          []validator.String{stringvalidator.OneOf("online", "offline", "pending")},
			},
			"filter": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Raw [PocketBase filter](https://pocketbase.io/docs/api-records/#listsearch-records), for example `ip ~ \"192.168.1.\"`.",
			},
			"devices": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Matching devices, sorted by name.",
				NestedObject:        schema.NestedAttributeObject{Attributes: deviceDataAttributes(false)},
			},
		},
	}
}

func (d *devicesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *devicesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg devicesDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var exprs []string
	if !cfg.Name.IsNull() {
		exprs = append(exprs, client.Eq("name", cfg.Name.ValueString()))
	}
	if !cfg.GroupID.IsNull() {
		exprs = append(exprs, client.AnyEq("groups.id", cfg.GroupID.ValueString()))
	}
	if !cfg.Status.IsNull() {
		exprs = append(exprs, client.Eq("status", cfg.Status.ValueString()))
	}
	if !cfg.Filter.IsNull() {
		exprs = append(exprs, cfg.Filter.ValueString())
	}
	devices, err := d.client.Devices.List(ctx, client.ListOptions{Filter: client.And(exprs...), Sort: "name,id"})
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "list devices", err)
		return
	}
	cfg.Devices = make([]deviceDataModel, len(devices))
	for i := range devices {
		cfg.Devices[i].fill(ctx, &devices[i], &resp.Diagnostics)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
