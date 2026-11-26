package provider

import (
	"os"
	"strings"
	"testing"

	"github.com/WindKube/terraform-provider-upsnap/internal/client"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func configureProvider(t *testing.T, attrs map[string]string) string {
	t.Helper()
	server, err := testAccProtoV6ProviderFactories["upsnap"]()
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	if _, err := server.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{}); err != nil {
		t.Fatal(err)
	}
	attrTypes := map[string]tftypes.Type{
		"endpoint": tftypes.String, "username": tftypes.String, "password": tftypes.String,
		"token": tftypes.String, "auth_collection": tftypes.String, "insecure_skip_verify": tftypes.Bool, "settle_time": tftypes.String,
	}
	objType := tftypes.Object{AttributeTypes: attrTypes}
	values := map[string]tftypes.Value{}
	for name, typ := range attrTypes {
		if v, ok := attrs[name]; ok {
			values[name] = tftypes.NewValue(typ, v)
		} else {
			values[name] = tftypes.NewValue(typ, nil)
		}
	}
	cfg, err := tfprotov6.NewDynamicValue(objType, tftypes.NewValue(objType, values))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := server.ConfigureProvider(ctx, &tfprotov6.ConfigureProviderRequest{TerraformVersion: "1.14.0", Config: &cfg})
	if err != nil {
		t.Fatal(err)
	}
	var s []string
	for _, d := range resp.Diagnostics {
		s = append(s, d.Summary+": "+d.Detail)
	}
	return strings.Join(s, "\n")
}

func TestAccProvider_configure(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	testAccPreCheck(t)
	endpoint, username, password := os.Getenv("UPSNAP_ENDPOINT"), os.Getenv("UPSNAP_USERNAME"), os.Getenv("UPSNAP_PASSWORD")
	t.Setenv("UPSNAP_ENDPOINT", "")
	t.Setenv("UPSNAP_USERNAME", "")
	t.Setenv("UPSNAP_PASSWORD", "")

	t.Run("password", func(t *testing.T) {
		if diags := configureProvider(t, map[string]string{"endpoint": endpoint, "username": username, "password": password}); diags != "" {
			t.Fatal(diags)
		}
	})

	t.Run("settle_time", func(t *testing.T) {
		base := map[string]string{"endpoint": endpoint, "username": username, "password": password}
		for _, v := range []string{"0s", "10s"} {
			base["settle_time"] = v
			if diags := configureProvider(t, base); diags != "" {
				t.Fatalf("%s: %s", v, diags)
			}
		}
		for _, v := range []string{"5", "-1s"} {
			base["settle_time"] = v
			if diags := configureProvider(t, base); !strings.Contains(diags, "Invalid settle_time") {
				t.Fatalf("%s: got %q", v, diags)
			}
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		diags := configureProvider(t, map[string]string{"endpoint": endpoint, "username": username, "password": "wrong-password"})
		if !strings.Contains(diags, "Unable to authenticate with UpSnap") {
			t.Fatalf("got %q", diags)
		}
	})

	t.Run("token", func(t *testing.T) {
		c, err := client.New(client.Config{Endpoint: endpoint, Username: username, Password: password})
		if err != nil {
			t.Fatal(err)
		}
		token, err := c.Token(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if diags := configureProvider(t, map[string]string{"endpoint": endpoint, "token": token}); diags != "" {
			t.Fatal(diags)
		}
		forged := token[:len(token)-4] + "AAAA"
		if diags := configureProvider(t, map[string]string{"endpoint": endpoint, "token": forged}); !strings.Contains(diags, "rejected") {
			t.Fatalf("got %q", diags)
		}
	})

	t.Run("environment", func(t *testing.T) {
		t.Setenv("UPSNAP_ENDPOINT", endpoint)
		t.Setenv("UPSNAP_USERNAME", username)
		t.Setenv("UPSNAP_PASSWORD", password)
		if diags := configureProvider(t, nil); diags != "" {
			t.Fatal(diags)
		}
	})

	t.Run("missing credentials", func(t *testing.T) {
		diags := configureProvider(t, map[string]string{"endpoint": endpoint})
		if !strings.Contains(diags, "Missing UpSnap credentials") {
			t.Fatalf("got %q", diags)
		}
	})

	t.Run("unreachable endpoint", func(t *testing.T) {
		diags := configureProvider(t, map[string]string{"endpoint": "http://127.0.0.1:1", "username": "a", "password": "b"})
		if !strings.Contains(diags, "Unable to authenticate with UpSnap") {
			t.Fatalf("got %q", diags)
		}
	})
}
