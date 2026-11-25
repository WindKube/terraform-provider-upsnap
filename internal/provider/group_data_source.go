package provider

import (
	"context"
	"errors"

	"github.com/WindKube/terraform-provider-upsnap/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSourceWithConfigure        = &groupDataSource{}
	_ datasource.DataSourceWithConfigValidators = &groupDataSource{}
	_ datasource.DataSourceWithConfigure        = &groupsDataSource{}
)

type groupDataModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	DeviceIDs types.Set    `tfsdk:"device_ids"`
}

func NewGroupDataSource() datasource.DataSource {
	return &groupDataSource{}
}

type groupDataSource struct {
	client *client.Client
}

func (d *groupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (d *groupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a device group by ID or by its unique name.",
		Attributes: map[string]schema.Attribute{
			"id":         schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Record ID. Exactly one of `id` and `name` must be set."},
			"name":       schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Group name. Fails when no group or more than one group has this name."},
			"device_ids": schema.SetAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "IDs of the devices in the group that the credentials can read."},
		},
	}
}

func (d *groupDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *groupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *groupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg groupDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var group *client.Group
	var err error
	if !cfg.ID.IsNull() {
		group, err = d.client.Groups.Get(ctx, cfg.ID.ValueString())
	} else {
		group, err = d.client.Groups.FindOne(ctx, client.Eq("name", cfg.Name.ValueString()))
	}
	switch {
	case client.IsNotFound(err):
		resp.Diagnostics.AddError("Group not found", "No group matches the given id or name.")
		return
	case errors.Is(err, client.ErrNotUnique):
		resp.Diagnostics.AddAttributeError(path.Root("name"), "Group name is not unique", "More than one group has this name; look it up by id instead.")
		return
	case err != nil:
		apiErrorDiag(&resp.Diagnostics, "read group", err)
		return
	}
	devices, err := d.client.Devices.List(ctx, client.ListOptions{Filter: client.AnyEq("groups.id", group.ID)})
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "list the devices of group "+group.ID, err)
		return
	}
	ids := make([]string, len(devices))
	for i, dev := range devices {
		ids[i] = dev.ID
	}
	cfg.ID, cfg.Name = types.StringValue(group.ID), types.StringValue(group.Name)
	cfg.DeviceIDs = stringSet(ctx, ids, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

func NewGroupsDataSource() datasource.DataSource {
	return &groupsDataSource{}
}

type groupsDataSource struct {
	client *client.Client
}

type groupItemModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

type groupsDataModel struct {
	Name   types.String     `tfsdk:"name"`
	Groups []groupItemModel `tfsdk:"groups"`
}

func (d *groupsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_groups"
}

func (d *groupsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "List device groups.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{Optional: true, MarkdownDescription: "Only groups with exactly this name."},
			"groups": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Matching groups, sorted by name.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":   schema.StringAttribute{Computed: true, MarkdownDescription: "Record ID."},
					"name": schema.StringAttribute{Computed: true, MarkdownDescription: "Group name."},
				}},
			},
		},
	}
}

func (d *groupsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *groupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg groupsDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	opts := client.ListOptions{Sort: "name,id"}
	if !cfg.Name.IsNull() {
		opts.Filter = client.Eq("name", cfg.Name.ValueString())
	}
	groups, err := d.client.Groups.List(ctx, opts)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "list groups", err)
		return
	}
	cfg.Groups = make([]groupItemModel, len(groups))
	for i, g := range groups {
		cfg.Groups[i] = groupItemModel{ID: types.StringValue(g.ID), Name: types.StringValue(g.Name)}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
