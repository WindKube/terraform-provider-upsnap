package provider

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/WindKube/terraform-provider-upsnap/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const defaultTimeout = 2 * time.Minute

func clientFromProviderData(data any, diags *diag.Diagnostics) *client.Client {
	if data == nil {
		return nil
	}
	c, ok := data.(*client.Client)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("Expected *client.Client, got %T. Please report this issue to the provider developers.", data))
		return nil
	}
	return c
}

type idIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

func idIdentitySchema(description string) identityschema.Schema {
	return identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				RequiredForImport: true,
				Description:       description,
			},
		},
	}
}

func setIDIdentity(ctx context.Context, identity interface {
	Set(context.Context, any) diag.Diagnostics
}, id string, diags *diag.Diagnostics) {
	if identity == nil || id == "" {
		return
	}
	diags.Append(identity.Set(ctx, idIdentityModel{ID: types.StringValue(id)})...)
}

type importLookup func(ctx context.Context, value string) (string, error)

// importByID passes a record ID through from either the import ID or the identity. Import IDs
// of the form "<prefix>:<value>" are resolved to a record ID with the matching lookup.
func importByID(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse, lookups map[string]importLookup) {
	if prefix, value, ok := strings.Cut(req.ID, ":"); ok {
		lookup, known := lookups[prefix]
		if !known {
			resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Unknown import ID prefix %q in %q. Import by record ID%s.", prefix, req.ID, importFormats(lookups)))
			return
		}
		id, err := lookup(ctx, value)
		if err != nil {
			resp.Diagnostics.AddError("Cannot import", importLookupError(prefix, value, err))
			return
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
		return
	}
	resource.ImportStatePassthroughWithIdentity(ctx, path.Root("id"), path.Root("id"), req, resp)
}

func importFormats(lookups map[string]importLookup) string {
	if len(lookups) == 0 {
		return ""
	}
	formats := make([]string, 0, len(lookups))
	for prefix := range lookups {
		formats = append(formats, fmt.Sprintf("%q", prefix+":<value>"))
	}
	slices.Sort(formats)
	return " or with " + strings.Join(formats, ", ")
}

func importLookupError(prefix, value string, err error) string {
	switch {
	case client.IsNotFound(err):
		return fmt.Sprintf("No record has %s %q.", prefix, value)
	case errors.Is(err, client.ErrNotUnique):
		return fmt.Sprintf("More than one record has %s %q; import by record ID instead.", prefix, value)
	default:
		return err.Error()
	}
}

func stringSet(ctx context.Context, values []string, diags *diag.Diagnostics) types.Set {
	if values == nil {
		values = []string{}
	}
	v, d := types.SetValueFrom(ctx, types.StringType, values)
	diags.Append(d...)
	return v
}

func setStrings(ctx context.Context, v types.Set, diags *diag.Diagnostics) []string {
	out := []string{}
	if v.IsNull() || v.IsUnknown() {
		return out
	}
	diags.Append(v.ElementsAs(ctx, &out, false)...)
	slices.Sort(out)
	return out
}

func int64OrNull(v int64) types.Int64 {
	if v == 0 {
		return types.Int64Null()
	}
	return types.Int64Value(v)
}

func apiErrorDiag(diags *diag.Diagnostics, action string, err error) {
	summary := "Unable to " + action
	if client.IsForbidden(err) {
		diags.AddError(summary, err.Error()+"\n\nThe configured credentials lack permission for this operation. Ports, users, permissions and settings require a PocketBase superuser.")
		return
	}
	diags.AddError(summary, err.Error())
}
