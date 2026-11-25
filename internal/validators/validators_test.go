package validators

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestChecks(t *testing.T) {
	cases := []struct {
		name  string
		check func(string) error
		valid []string
		bad   []string
	}{
		{"mac", CheckMAC,
			[]string{"aa:bb:cc:dd:ee:ff", "AA-BB-CC-DD-EE-FF", "00:11:22:33:44:55"},
			[]string{"aa:bb:cc:dd:ee", "aa:bb:cc:dd:ee:fg", "aabb.ccdd.eeff", "aa:bb-cc:dd:ee:ff", "aa:bb:cc:dd:ee:ff:00"}},
		{"host", CheckHost,
			[]string{"192.168.1.10", "0.0.0.0", "nas.lan", "pc-01.home.arpa", "büro.example"},
			[]string{"256.1.1.1", "192.168.1", "localhost", "-bad.lan", "has space.lan", "192.168.01.1"}},
		{"netmask", CheckNetmask,
			[]string{"255.255.255.255", "255.255.255.0", "255.255.240.0", "0.0.0.0", "128.0.0.0"},
			[]string{"255.0.255.0", "255.255.255.1", "1.2.3.4", "ffff::", "garbage"}},
		{"cron", CheckCron,
			[]string{"0 8 * * 1-5", "*/3 * * * * *", "30 0 22 * * *", "0 0 1 1 *"},
			[]string{"* * * *", "61 * * * *", "@every 1m", "not cron", "* * * * * * *"}},
		{"cidr", CheckCIDR,
			[]string{"192.168.1.0/24", "10.0.0.0/8", "fd00::/64"},
			[]string{"192.168.1.0", "192.168.1.0/33", "net"}},
		{"record id", CheckRecordID,
			[]string{"z5lghx2r3tm45n1", "aaaaaaaaaaaaaaa"},
			[]string{"short", "Z5LGHX2R3TM45N1", "z5lghx2r3tm45n1x", "z5lghx2r3tm45-1"}},
	}
	for _, tc := range cases {
		for _, v := range tc.valid {
			if err := tc.check(v); err != nil {
				t.Errorf("%s: %q should be valid: %v", tc.name, v, err)
			}
		}
		for _, v := range tc.bad {
			if err := tc.check(v); err == nil {
				t.Errorf("%s: %q should be invalid", tc.name, v)
			}
		}
	}
}

func TestValidatorSkipsNullUnknownAndEmpty(t *testing.T) {
	for _, v := range []types.String{types.StringNull(), types.StringUnknown(), types.StringValue("")} {
		resp := &validator.StringResponse{}
		MAC().ValidateString(context.Background(), validator.StringRequest{Path: path.Root("mac"), ConfigValue: v}, resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("%v: unexpected error %v", v, resp.Diagnostics)
		}
	}
}

func TestValidatorReportsAttributePath(t *testing.T) {
	resp := &validator.StringResponse{}
	Netmask().ValidateString(context.Background(), validator.StringRequest{Path: path.Root("netmask"), ConfigValue: types.StringValue("255.0.255.0")}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
	if got := resp.Diagnostics.Errors()[0].Detail(); got != `"255.0.255.0" is not a contiguous IPv4 subnet mask` {
		t.Fatalf("got %q", got)
	}
}
