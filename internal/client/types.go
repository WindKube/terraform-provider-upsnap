package client

type Device struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	IP                  string   `json:"ip"`
	MAC                 string   `json:"mac"`
	Netmask             string   `json:"netmask"`
	Description         string   `json:"description"`
	Link                string   `json:"link"`
	LinkOpen            string   `json:"link_open"`
	TrackIP             bool     `json:"track_ip"`
	Ports               []string `json:"ports"`
	Groups              []string `json:"groups"`
	CreatedBy           string   `json:"created_by"`
	Status              string   `json:"status"`
	WakeCron            string   `json:"wake_cron"`
	WakeCronEnabled     bool     `json:"wake_cron_enabled"`
	WakeCmd             string   `json:"wake_cmd"`
	WakeConfirm         bool     `json:"wake_confirm"`
	WakeTimeout         int64    `json:"wake_timeout"`
	ShutdownCron        string   `json:"shutdown_cron"`
	ShutdownCronEnabled bool     `json:"shutdown_cron_enabled"`
	ShutdownCmd         string   `json:"shutdown_cmd"`
	ShutdownConfirm     bool     `json:"shutdown_confirm"`
	ShutdownTimeout     int64    `json:"shutdown_timeout"`
	PingCmd             string   `json:"ping_cmd"`
	Password            string   `json:"password"`
	SOLEnabled          bool     `json:"sol_enabled"`
	SOLAuth             bool     `json:"sol_auth"`
	SOLUser             string   `json:"sol_user"`
	SOLPassword         string   `json:"sol_password"`
	SOLPort             int64    `json:"sol_port"`
	Created             string   `json:"created"`
	Updated             string   `json:"updated"`
}

func (d *Device) UpdatedAt() string { return d.Updated }

// DeviceInput holds the writable device fields. Ports are linked by the port endpoints
// and status is owned by UpSnap, so neither is sent.
type DeviceInput struct {
	Name                string   `json:"name"`
	IP                  string   `json:"ip"`
	MAC                 string   `json:"mac"`
	Netmask             string   `json:"netmask"`
	Description         string   `json:"description"`
	Link                string   `json:"link"`
	LinkOpen            string   `json:"link_open"`
	TrackIP             bool     `json:"track_ip"`
	Groups              []string `json:"groups"`
	CreatedBy           string   `json:"created_by"`
	WakeCron            string   `json:"wake_cron"`
	WakeCronEnabled     bool     `json:"wake_cron_enabled"`
	WakeCmd             string   `json:"wake_cmd"`
	WakeConfirm         bool     `json:"wake_confirm"`
	WakeTimeout         int64    `json:"wake_timeout"`
	ShutdownCron        string   `json:"shutdown_cron"`
	ShutdownCronEnabled bool     `json:"shutdown_cron_enabled"`
	ShutdownCmd         string   `json:"shutdown_cmd"`
	ShutdownConfirm     bool     `json:"shutdown_confirm"`
	ShutdownTimeout     int64    `json:"shutdown_timeout"`
	PingCmd             string   `json:"ping_cmd"`
	Password            string   `json:"password"`
	SOLEnabled          bool     `json:"sol_enabled"`
	SOLAuth             bool     `json:"sol_auth"`
	SOLUser             string   `json:"sol_user"`
	SOLPassword         string   `json:"sol_password"`
	SOLPort             int64    `json:"sol_port"`
}

type Port struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Number  int64  `json:"number"`
	Link    string `json:"link"`
	Status  bool   `json:"status"`
	Updated string `json:"updated"`
}

func (p *Port) UpdatedAt() string { return p.Updated }

type PortInput struct {
	Name   string `json:"name"`
	Number int64  `json:"number"`
	Link   string `json:"link"`
}

type Group struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type GroupInput struct {
	Name string `json:"name"`
}

type User struct {
	ID              string `json:"id"`
	Username        string `json:"username"`
	Email           string `json:"email"`
	EmailVisibility bool   `json:"emailVisibility"`
	Verified        bool   `json:"verified"`
	Avatar          int64  `json:"avatar"`
}

type UserInput struct {
	Username        string `json:"username"`
	Email           string `json:"email"`
	EmailVisibility bool   `json:"emailVisibility"`
	Verified        bool   `json:"verified"`
	Avatar          int64  `json:"avatar"`
	Password        string `json:"password,omitempty"`
	PasswordConfirm string `json:"passwordConfirm,omitempty"`
}

type Permission struct {
	ID     string   `json:"id"`
	User   string   `json:"user"`
	Create bool     `json:"create"`
	Read   []string `json:"read"`
	Update []string `json:"update"`
	Delete []string `json:"delete"`
	Power  []string `json:"power"`
}

type PermissionInput struct {
	User   string   `json:"user"`
	Create bool     `json:"create"`
	Read   []string `json:"read"`
	Update []string `json:"update"`
	Delete []string `json:"delete"`
	Power  []string `json:"power"`
}

type SettingsPrivate struct {
	ID              string `json:"id"`
	Interval        string `json:"interval"`
	ScanRange       string `json:"scan_range"`
	LazyPing        bool   `json:"lazy_ping"`
	TrackIPInterval string `json:"track_ip_interval"`
}

type SettingsPublic struct {
	ID             string `json:"id"`
	WebsiteTitle   string `json:"website_title"`
	SetupCompleted bool   `json:"setup_completed"`
}
