package client

import (
	"context"
	"net/http"
	"net/url"
)

type PowerAction string

const (
	ActionWake     PowerAction = "wake"
	ActionSleep    PowerAction = "sleep"
	ActionReboot   PowerAction = "reboot"
	ActionShutdown PowerAction = "shutdown"
)

type GroupPowerAction string

const (
	GroupActionWake     GroupPowerAction = "wakegroup"
	GroupActionShutdown GroupPowerAction = "shutdowngroup"
)

// Power runs a power action against a device. With wait UpSnap answers once the device
// reached the target state (or the device timeout elapsed); otherwise it returns at once.
func (c *Client) Power(ctx context.Context, action PowerAction, deviceID string, wait bool) error {
	q := url.Values{}
	if !wait {
		q.Set("async", "true")
	}
	return c.do(ctx, http.MethodGet, "/api/upsnap/"+string(action)+"/"+url.PathEscape(deviceID), q, nil, nil)
}

// GroupPower starts a power action for every matching device of a group and returns the
// devices it was started for. UpSnap always runs group actions in the background.
func (c *Client) GroupPower(ctx context.Context, action GroupPowerAction, groupID string) ([]Device, error) {
	var devices []Device
	err := c.do(ctx, http.MethodGet, "/api/upsnap/"+string(action)+"/"+url.PathEscape(groupID), nil, nil, &devices)
	return devices, err
}

// ValidateCron asks UpSnap whether expr is a cron expression it accepts.
func (c *Client) ValidateCron(ctx context.Context, expr string) error {
	return c.doWithAuth(ctx, http.MethodPost, "/api/upsnap/validate-cron", nil, map[string]string{"cron": expr}, nil, false)
}
