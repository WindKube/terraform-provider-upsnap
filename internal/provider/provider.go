package provider

import (
	"context"
	"os"
	"strconv"

	"github.com/WindKube/terraform-provider-upsnap/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ provider.Provider            = &UpSnapProvider{}
	_ provider.ProviderWithActions = &UpSnapProvider{}
)

type UpSnapProvider struct {
	version string
}

type providerModel struct {
	Endpoint           types.String `tfsdk:"endpoint"`
	Username           types.String `tfsdk:"username"`
	Password           types.String `tfsdk:"password"`
	Token              types.String `tfsdk:"token"`
	AuthCollection     types.String `tfsdk:"auth_collection"`
	InsecureSkipVerify types.Bool   `tfsdk:"insecure_skip_verify"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &UpSnapProvider{version: version}
	}
}

func (p *UpSnapProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "upsnap"
	resp.Version = p.version
}

func (p *UpSnapProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage [UpSnap](https://github.com/seriousm4x/UpSnap) wake-on-LAN dashboards through their PocketBase API.\n\n" +
			"Authenticate either with `username` and `password` (a PocketBase superuser by default, or an UpSnap user with `auth_collection = \"users\"`) or with a `token`. " +
			"Ports, users, permissions and settings can only be managed by a superuser.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "Base URL of UpSnap, for example `http://upsnap.lan:8090`. Can also be set with `UPSNAP_ENDPOINT`.",
				Optional:            true,
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "Superuser email or UpSnap username/email. Can also be set with `UPSNAP_USERNAME`.",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("token")),
					stringvalidator.AlsoRequires(path.MatchRoot("password")),
				},
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Password for `username`. Can also be set with `UPSNAP_PASSWORD`.",
				Optional:            true,
				Sensitive:           true,
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("token")),
				},
			},
			"token": schema.StringAttribute{
				MarkdownDescription: "PocketBase auth token, for example a superuser impersonation token. Refreshable tokens are refreshed before they expire. Can also be set with `UPSNAP_TOKEN`.",
				Optional:            true,
				Sensitive:           true,
			},
			"auth_collection": schema.StringAttribute{
				MarkdownDescription: "Collection `username` authenticates against: `_superusers` (default) or `users`. Can also be set with `UPSNAP_AUTH_COLLECTION`.",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(client.SuperusersCollection, client.UsersCollection),
				},
			},
			"insecure_skip_verify": schema.BoolAttribute{
				MarkdownDescription: "Skip TLS certificate verification. Can also be set with `UPSNAP_INSECURE_SKIP_VERIFY`.",
				Optional:            true,
			},
		},
	}
}

func (p *UpSnapProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for name, v := range map[string]interface{ IsUnknown() bool }{
		"endpoint": cfg.Endpoint, "username": cfg.Username, "password": cfg.Password, "token": cfg.Token,
		"auth_collection": cfg.AuthCollection, "insecure_skip_verify": cfg.InsecureSkipVerify,
	} {
		if v.IsUnknown() {
			resp.Diagnostics.AddAttributeError(path.Root(name), "Unknown UpSnap provider configuration",
				"The provider cannot be configured with a value that is only known after apply. Set it statically or through its environment variable.")
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint := stringWithEnv(cfg.Endpoint, "UPSNAP_ENDPOINT")
	authCollection := stringWithEnv(cfg.AuthCollection, "UPSNAP_AUTH_COLLECTION")
	username := stringWithEnv(cfg.Username, "UPSNAP_USERNAME")
	password := stringWithEnv(cfg.Password, "UPSNAP_PASSWORD")
	token := stringWithEnv(cfg.Token, "UPSNAP_TOKEN")
	switch {
	case !cfg.Token.IsNull():
		username, password = "", ""
	case username != "" && password != "":
		token = ""
	}

	insecure := cfg.InsecureSkipVerify.ValueBool()
	if cfg.InsecureSkipVerify.IsNull() {
		if v := os.Getenv("UPSNAP_INSECURE_SKIP_VERIFY"); v != "" {
			b, err := strconv.ParseBool(v)
			if err != nil {
				resp.Diagnostics.AddAttributeError(path.Root("insecure_skip_verify"), "Invalid UPSNAP_INSECURE_SKIP_VERIFY", err.Error())
				return
			}
			insecure = b
		}
	}

	if endpoint == "" {
		resp.Diagnostics.AddAttributeError(path.Root("endpoint"), "Missing UpSnap endpoint",
			"Set the endpoint argument or the UPSNAP_ENDPOINT environment variable.")
	}
	if authCollection != "" && authCollection != client.SuperusersCollection && authCollection != client.UsersCollection {
		resp.Diagnostics.AddAttributeError(path.Root("auth_collection"), "Invalid auth collection",
			"auth_collection (or UPSNAP_AUTH_COLLECTION) must be _superusers or users.")
	}
	if token == "" && (username == "" || password == "") {
		resp.Diagnostics.AddError("Missing UpSnap credentials",
			"Set username and password (UPSNAP_USERNAME/UPSNAP_PASSWORD) or token (UPSNAP_TOKEN).")
	}
	if resp.Diagnostics.HasError() {
		return
	}

	c, err := client.New(client.Config{
		Endpoint:           endpoint,
		Username:           username,
		Password:           password,
		Token:              token,
		AuthCollection:     authCollection,
		InsecureSkipVerify: insecure,
		UserAgent:          "terraform-provider-upsnap/" + p.version,
	})
	if err != nil {
		resp.Diagnostics.AddError("Invalid UpSnap provider configuration", err.Error())
		return
	}
	if err := c.Authenticate(ctx); err != nil {
		resp.Diagnostics.AddError("Unable to authenticate with UpSnap", err.Error())
		return
	}

	resp.DataSourceData = c
	resp.ResourceData = c
	resp.ActionData = c
}

func stringWithEnv(v types.String, env string) string {
	if !v.IsNull() {
		return v.ValueString()
	}
	return os.Getenv(env)
}

func (p *UpSnapProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewDeviceResource,
		NewPortResource,
	}
}

func (p *UpSnapProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewDeviceDataSource,
		NewDevicesDataSource,
	}
}

func (p *UpSnapProvider) Actions(_ context.Context) []func() action.Action {
	return nil
}
