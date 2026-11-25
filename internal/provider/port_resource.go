package provider

import (
	"context"
	"slices"

	"github.com/WindKube/terraform-provider-upsnap/internal/client"
	"github.com/WindKube/terraform-provider-upsnap/internal/validators"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.ResourceWithConfigure   = &portResource{}
	_ resource.ResourceWithImportState = &portResource{}
	_ resource.ResourceWithIdentity    = &portResource{}
)

func NewPortResource() resource.Resource {
	return &portResource{}
}

type portResource struct {
	client *client.Client
}

type portModel struct {
	ID       types.String   `tfsdk:"id"`
	DeviceID types.String   `tfsdk:"device_id"`
	Name     types.String   `tfsdk:"name"`
	Number   types.Int64    `tfsdk:"number"`
	Link     types.String   `tfsdk:"link"`
	Timeouts timeouts.Value `tfsdk:"timeouts"`
}

func (r *portResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_port"
}

func (r *portResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A TCP port UpSnap checks on a device (`ports` collection), shown as open or closed on the device card.\n\n" +
			"The port record is linked to its device through the device's `ports` relation. Requires superuser credentials.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Record ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"device_id": schema.StringAttribute{
				MarkdownDescription: "ID of the `upsnap_device` the port belongs to. Changing it moves the port in place.",
				Required:            true,
				Validators:          []validator.String{validators.RecordID()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name, for example `SSH`.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"number": schema.Int64Attribute{
				MarkdownDescription: "Port number, 1-65535.",
				Required:            true,
				Validators:          []validator.Int64{int64validator.Between(1, 65535)},
			},
			"link": optionalString("URL opened when the port is clicked."),
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true, Read: true, Update: true, Delete: true,
			}),
		},
	}
}

func (r *portResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = idIdentitySchema("Port record ID.")
}

func (r *portResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (m *portModel) input() client.PortInput {
	return client.PortInput{Name: m.Name.ValueString(), Number: m.Number.ValueInt64(), Link: m.Link.ValueString()}
}

func (m *portModel) fill(p *client.Port, deviceID string) {
	m.ID = types.StringValue(p.ID)
	m.Name = types.StringValue(p.Name)
	m.Number = types.Int64Value(p.Number)
	m.Link = types.StringValue(p.Link)
	m.DeviceID = types.StringValue(deviceID)
}

func (r *portResource) link(ctx context.Context, deviceID, portID string) error {
	_, err := r.client.Devices.Update(ctx, deviceID, map[string]any{"ports+": portID})
	return err
}

func (r *portResource) unlink(ctx context.Context, deviceID, portID string) error {
	_, err := r.client.Devices.Update(ctx, deviceID, map[string]any{"ports-": portID})
	if client.IsNotFound(err) {
		return nil
	}
	return err
}

// owner returns the device whose ports relation holds portID, preferring want when several do.
func (r *portResource) owner(ctx context.Context, portID, want string) (string, error) {
	devices, err := r.client.Devices.List(ctx, client.ListOptions{Filter: client.AnyEq("ports.id", portID), Sort: "id"})
	if err != nil || len(devices) == 0 {
		return "", err
	}
	if i := slices.IndexFunc(devices, func(d client.Device) bool { return d.ID == want }); i >= 0 {
		return devices[i].ID, nil
	}
	return devices[0].ID, nil
}

func (r *portResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan portModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := plan.Timeouts.Create(ctx, defaultTimeout)
	resp.Diagnostics.Append(diags...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	deviceID := plan.DeviceID.ValueString()
	if _, err := r.client.Devices.Get(ctx, deviceID); err != nil {
		apiErrorDiag(&resp.Diagnostics, "find device "+deviceID+" for the port", err)
		return
	}
	port, err := r.client.Ports.Create(ctx, plan.input())
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "create port", err)
		return
	}
	if err := r.link(ctx, deviceID, port.ID); err != nil {
		apiErrorDiag(&resp.Diagnostics, "link port to device "+deviceID, err)
		if delErr := r.client.Ports.Delete(ctx, port.ID); delErr != nil {
			resp.Diagnostics.AddWarning("Orphaned port record", "Port "+port.ID+" could not be removed after linking failed: "+delErr.Error())
		}
		return
	}
	plan.fill(port, deviceID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	setIDIdentity(ctx, resp.Identity, port.ID, &resp.Diagnostics)
}

func (r *portResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state portModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := state.Timeouts.Read(ctx, defaultTimeout)
	resp.Diagnostics.Append(diags...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	port, err := r.client.Ports.Get(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "read port", err)
		return
	}
	deviceID, err := r.owner(ctx, port.ID, state.DeviceID.ValueString())
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "find the device of port "+port.ID, err)
		return
	}
	state.fill(port, deviceID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	setIDIdentity(ctx, resp.Identity, port.ID, &resp.Diagnostics)
}

func (r *portResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state portModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := plan.Timeouts.Update(ctx, defaultTimeout)
	resp.Diagnostics.Append(diags...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	port, err := r.client.Ports.Update(ctx, state.ID.ValueString(), plan.input())
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "update port", err)
		return
	}
	r.relink(ctx, port.ID, state.DeviceID.ValueString(), plan.DeviceID.ValueString(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.fill(port, plan.DeviceID.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	setIDIdentity(ctx, resp.Identity, port.ID, &resp.Diagnostics)
}

func (r *portResource) relink(ctx context.Context, portID, from, to string, diags *diag.Diagnostics) {
	if from == to {
		return
	}
	if err := r.link(ctx, to, portID); err != nil {
		apiErrorDiag(diags, "link port to device "+to, err)
		return
	}
	if from != "" {
		if err := r.unlink(ctx, from, portID); err != nil {
			apiErrorDiag(diags, "unlink port from device "+from, err)
		}
	}
}

func (r *portResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state portModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := state.Timeouts.Delete(ctx, defaultTimeout)
	resp.Diagnostics.Append(diags...)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := r.client.Ports.Delete(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		apiErrorDiag(&resp.Diagnostics, "delete port", err)
	}
}

func (r *portResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByID(ctx, req, resp, "", nil)
}
