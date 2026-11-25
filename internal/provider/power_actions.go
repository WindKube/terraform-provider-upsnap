package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/WindKube/terraform-provider-upsnap/internal/client"
	"github.com/WindKube/terraform-provider-upsnap/internal/validators"
	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const defaultActionTimeout = 5 * time.Minute

var (
	_ action.ActionWithConfigure = &devicePowerAction{}
	_ action.ActionWithConfigure = &groupPowerAction{}
)

type devicePowerKind struct {
	name        string
	verb        string
	api         client.PowerAction
	description string
}

var devicePowerKinds = []devicePowerKind{
	{"wake", "Waking", client.ActionWake,
		"Wake a device with a magic packet, or its `wake_cmd`. With `wait` the action finishes once UpSnap sees the device online, and fails if it does not come online within the device's `wake_timeout`."},
	{"shutdown", "Shutting down", client.ActionShutdown,
		"Shut a device down with its `shutdown_cmd` or Sleep-On-LAN. With `wait` the action finishes once UpSnap sees the device offline, and fails if it is still online after the device's `shutdown_timeout`."},
	{"sleep", "Suspending", client.ActionSleep,
		"Suspend a device through [Sleep-On-LAN](https://github.com/SR-G/sleep-on-lan); requires `sol_enabled` on the device."},
	{"reboot", "Rebooting", client.ActionReboot,
		"Reboot a device: UpSnap shuts it down, waits 15 seconds, then wakes it. With `wait` the action finishes once the device is back online."},
}

func deviceActionFactories() []func() action.Action {
	out := make([]func() action.Action, 0, len(devicePowerKinds))
	for _, kind := range devicePowerKinds {
		out = append(out, func() action.Action { return &devicePowerAction{kind: kind} })
	}
	return out
}

type devicePowerAction struct {
	client *client.Client
	kind   devicePowerKind
}

type devicePowerModel struct {
	DeviceID types.String `tfsdk:"device_id"`
	Wait     types.Bool   `tfsdk:"wait"`
	Timeout  types.String `tfsdk:"timeout"`
}

func (a *devicePowerAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_device_" + a.kind.name
}

func timeoutAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: "How long to wait for UpSnap, as a Go duration such as `90s` or `10m`. Defaults to `5m`.",
		Optional:            true,
		Validators:          []validator.String{durationValidator{}},
	}
}

func (a *devicePowerAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: a.kind.description + "\n\nRequires Terraform 1.14 or later. The credentials need `power` permission on the device, or a superuser.",
		Attributes: map[string]schema.Attribute{
			"device_id": schema.StringAttribute{
				MarkdownDescription: "ID of the device.",
				Required:            true,
				Validators:          []validator.String{validators.RecordID()},
			},
			"wait": schema.BoolAttribute{
				MarkdownDescription: "Wait until UpSnap reports the result. When `false`, UpSnap runs the action in the background and the action returns at once. Defaults to `true`.",
				Optional:            true,
			},
			"timeout": timeoutAttribute(),
		},
	}
}

func (a *devicePowerAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (a *devicePowerAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var cfg devicePowerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, parseTimeout(cfg.Timeout))
	defer cancel()

	id := cfg.DeviceID.ValueString()
	device, err := a.client.Devices.Get(ctx, id)
	if err != nil {
		if client.IsNotFound(err) {
			resp.Diagnostics.AddAttributeError(path.Root("device_id"), "Device not found", "No device with ID "+id+" exists, or the credentials cannot read it.")
			return
		}
		apiErrorDiag(&resp.Diagnostics, "read device "+id, err)
		return
	}

	wait := cfg.Wait.IsNull() || cfg.Wait.ValueBool()
	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("%s %s (%s)", a.kind.verb, device.Name, id)})
	if err := a.client.Power(ctx, a.kind.api, id, wait); err != nil {
		if ctx.Err() != nil {
			resp.Diagnostics.AddError("Timed out", fmt.Sprintf("%s %s did not finish within the timeout: %s", a.kind.verb, device.Name, err))
			return
		}
		apiErrorDiag(&resp.Diagnostics, a.kind.name+" device "+device.Name, err)
		return
	}
	if !wait {
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("UpSnap is %s %s in the background", strings.ToLower(a.kind.verb), device.Name)})
		return
	}
	if after, err := a.client.Devices.Get(ctx, id); err == nil {
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("%s is %s", device.Name, after.Status)})
	}
}

type groupPowerKind struct {
	name        string
	verb        string
	api         client.GroupPowerAction
	description string
}

var groupPowerKinds = []groupPowerKind{
	{"wake", "Waking", client.GroupActionWake, "Wake every offline device in a group."},
	{"shutdown", "Shutting down", client.GroupActionShutdown, "Shut down every online device in a group."},
}

func groupActionFactories() []func() action.Action {
	out := make([]func() action.Action, 0, len(groupPowerKinds))
	for _, kind := range groupPowerKinds {
		out = append(out, func() action.Action { return &groupPowerAction{kind: kind} })
	}
	return out
}

type groupPowerAction struct {
	client *client.Client
	kind   groupPowerKind
}

type groupPowerModel struct {
	GroupID types.String `tfsdk:"group_id"`
	Timeout types.String `tfsdk:"timeout"`
}

func (a *groupPowerAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_" + a.kind.name
}

func (a *groupPowerAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: a.kind.description + " UpSnap starts the device actions in the background, so the action returns once they are started.\n\n" +
			"Requires Terraform 1.14 or later. The credentials need `power` permission on the group's devices, or a superuser.",
		Attributes: map[string]schema.Attribute{
			"group_id": schema.StringAttribute{
				MarkdownDescription: "ID of the group.",
				Required:            true,
				Validators:          []validator.String{validators.RecordID()},
			},
			"timeout": timeoutAttribute(),
		},
	}
}

func (a *groupPowerAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (a *groupPowerAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var cfg groupPowerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, parseTimeout(cfg.Timeout))
	defer cancel()

	id := cfg.GroupID.ValueString()
	group, err := a.client.Groups.Get(ctx, id)
	if err != nil {
		if client.IsNotFound(err) {
			resp.Diagnostics.AddAttributeError(path.Root("group_id"), "Group not found", "No group with ID "+id+" exists.")
			return
		}
		apiErrorDiag(&resp.Diagnostics, "read group "+id, err)
		return
	}
	devices, err := a.client.GroupPower(ctx, a.kind.api, id)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, a.kind.name+" group "+group.Name, err)
		return
	}
	if len(devices) == 0 {
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("No device in %s needed %s", group.Name, a.kind.name)})
		return
	}
	names := make([]string, len(devices))
	for i, d := range devices {
		names[i] = d.Name
	}
	resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf("%s %s: %s", a.kind.verb, group.Name, strings.Join(names, ", "))})
}

func parseTimeout(v types.String) time.Duration {
	if v.IsNull() || v.IsUnknown() {
		return defaultActionTimeout
	}
	d, err := time.ParseDuration(v.ValueString())
	if err != nil || d <= 0 {
		return defaultActionTimeout
	}
	return d
}

type durationValidator struct{}

func (durationValidator) Description(_ context.Context) string {
	return "value must be a positive Go duration such as 90s or 10m"
}

func (v durationValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (durationValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if d, err := time.ParseDuration(req.ConfigValue.ValueString()); err != nil || d <= 0 {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid duration", fmt.Sprintf("%q is not a positive Go duration such as 90s or 10m.", req.ConfigValue.ValueString()))
	}
}
