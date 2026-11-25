package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// requiredWhenTrue requires a non-empty string attribute whenever a bool flag is true.
type requiredWhenTrue struct {
	flag     string
	required string
}

var _ resource.ConfigValidator = requiredWhenTrue{}

func (v requiredWhenTrue) Description(_ context.Context) string {
	return fmt.Sprintf("%s must be set when %s is true", v.required, v.flag)
}

func (v requiredWhenTrue) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v requiredWhenTrue) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var flag types.Bool
	var required types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(v.flag), &flag)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(v.required), &required)...)
	if resp.Diagnostics.HasError() || flag.IsUnknown() || required.IsUnknown() {
		return
	}
	if flag.ValueBool() && required.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(path.Root(v.required), "Missing required attribute",
			fmt.Sprintf("%s must be set when %s is true.", v.required, v.flag))
	}
}
