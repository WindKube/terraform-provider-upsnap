// Package validators holds string validators that mirror UpSnap's own field rules, so invalid
// values fail at plan time instead of being rejected or silently rewritten by the server.
package validators

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/robfig/cron/v3"
)

// Patterns copied from backend/migrations at UpSnap 5.7.3.
var (
	macPattern  = regexp.MustCompile(`^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$`)
	hostPattern = regexp.MustCompile(`^(?:(?:[\p{L}\p{N}](?:[\p{L}\p{N}-]{0,61}[\p{L}\p{N}])?\.)+[\p{L}\p{N}](?:[\p{L}\p{N}-]{0,61}[\p{L}\p{N}])?|(?:25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(?:\.(?:25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3})$`)

	cronParser = cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
)

type stringFunc struct {
	description string
	check       func(string) error
}

func (v stringFunc) Description(_ context.Context) string         { return v.description }
func (v stringFunc) MarkdownDescription(_ context.Context) string { return v.description }

func (v stringFunc) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() || req.ConfigValue.ValueString() == "" {
		return
	}
	if err := v.check(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid value", fmt.Sprintf("%q %s", req.ConfigValue.ValueString(), err))
	}
}

// MAC accepts six colon- or dash-separated hex octets with a consistent separator.
func MAC() validator.String {
	return stringFunc{"value must be a MAC address such as aa:bb:cc:dd:ee:ff", CheckMAC}
}

func CheckMAC(s string) error {
	if !macPattern.MatchString(s) {
		return fmt.Errorf("is not a MAC address such as aa:bb:cc:dd:ee:ff")
	}
	if _, err := net.ParseMAC(s); err != nil {
		return fmt.Errorf("is not a MAC address: %w", err)
	}
	return nil
}

// Host accepts an IPv4 address or a fully qualified host name, as UpSnap does.
func Host() validator.String {
	return stringFunc{"value must be an IPv4 address or a fully qualified host name", CheckHost}
}

// UpSnap's pattern also matches all-numeric "host names" such as 256.1.1.1; those are rejected
// unless they are real IPv4 addresses.
func CheckHost(s string) error {
	if !hostPattern.MatchString(s) {
		return fmt.Errorf("is not an IPv4 address or a fully qualified host name (UpSnap requires a dot in host names)")
	}
	if numericHostPattern.MatchString(s) {
		if addr, err := netip.ParseAddr(s); err != nil || !addr.Is4() {
			return fmt.Errorf("is not a valid IPv4 address")
		}
	}
	return nil
}

var numericHostPattern = regexp.MustCompile(`^[0-9.]+$`)

// Netmask accepts contiguous IPv4 subnet masks. UpSnap silently replaces any other value
// with 255.255.255.255, which would otherwise surface as an inconsistent apply result.
func Netmask() validator.String {
	return stringFunc{"value must be a contiguous IPv4 subnet mask such as 255.255.255.0", CheckNetmask}
}

func CheckNetmask(s string) error {
	addr, err := netip.ParseAddr(s)
	if err != nil || !addr.Is4() {
		return fmt.Errorf("is not an IPv4 subnet mask")
	}
	b := addr.As4()
	inverted := ^(uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3]))
	if inverted&(inverted+1) != 0 {
		return fmt.Errorf("is not a contiguous IPv4 subnet mask")
	}
	return nil
}

// Cron accepts the expressions UpSnap accepts: five fields, or six with leading seconds.
func Cron() validator.String {
	return stringFunc{"value must be a cron expression with five fields, or six with leading seconds", CheckCron}
}

func CheckCron(s string) error {
	if _, err := cronParser.Parse(s); err != nil {
		return fmt.Errorf("is not a valid cron expression: %w", err)
	}
	return nil
}

// CIDR accepts an IP network in CIDR notation.
func CIDR() validator.String {
	return stringFunc{"value must be a network in CIDR notation such as 192.168.1.0/24", CheckCIDR}
}

func CheckCIDR(s string) error {
	if _, _, err := net.ParseCIDR(s); err != nil {
		return fmt.Errorf("is not a network in CIDR notation")
	}
	return nil
}

// RecordID accepts PocketBase record IDs: 15 lowercase alphanumeric characters.
func RecordID() validator.String {
	return stringFunc{"value must be a PocketBase record ID (15 lowercase letters and digits)", CheckRecordID}
}

var recordIDPattern = regexp.MustCompile(`^[a-z0-9]{15}$`)

func CheckRecordID(s string) error {
	if !recordIDPattern.MatchString(s) {
		return fmt.Errorf("is not a PocketBase record ID (15 lowercase letters and digits)")
	}
	return nil
}
