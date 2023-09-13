package xcore

// ReqRegister is the JSON request
type ReqRegister struct {
	Username                 string      `json:"username,omitempty"`
	BindEmail                bool        `json:"bind_email,omitempty"`
	Password                 string      `json:"password,omitempty"`
	DeviceID                 string      `json:"device_id,omitempty"`
	InitialDeviceDisplayName string      `json:"initial_device_display_name"`
	Auth                     interface{} `json:"auth,omitempty"`
}

// ReqLogin is the JSON request
type ReqLogin struct {
	Type                     string     `json:"type"`
	Identifier               Identifier `json:"identifier,omitempty"`
	Password                 string     `json:"password,omitempty"`
	Medium                   string     `json:"medium,omitempty"`
	User                     string     `json:"user,omitempty"`
	Address                  string     `json:"address,omitempty"`
	Token                    string     `json:"token,omitempty"`
	DeviceID                 string     `json:"device_id,omitempty"`
	InitialDeviceDisplayName string     `json:"initial_device_display_name,omitempty"`
}

// ReqCreateFrame is the JSON request
type ReqCreateFrame struct {
	Visibility      string                 `json:"visibility,omitempty"`
	FrameAliasName   string                 `json:"frame_alias_name,omitempty"`
	Name            string                 `json:"name,omitempty"`
	Topic           string                 `json:"topic,omitempty"`
	Invite          []string               `json:"invite,omitempty"`
	Invite3PID      []ReqInvite3PID        `json:"invite_3pid,omitempty"`
	CreationContent map[string]interface{} `json:"creation_content,omitempty"`
	InitialState    []Event                `json:"initial_state,omitempty"`
	Preset          string                 `json:"preset,omitempty"`
	IsDirect        bool                   `json:"is_direct,omitempty"`
}

// ReqRedact is the JSON request
type ReqRedact struct {
	Reason string `json:"reason,omitempty"`
}

// ReqInvite3PID is the JSON request
// It is also a JSON object
type ReqInvite3PID struct {
	IDServer string `json:"id_server"`
	Medium   string `json:"medium"`
	Address  string `json:"address"`
}

// ReqInviteUser is the JSON request
type ReqInviteUser struct {
	UserID string `json:"user_id"`
}

// ReqKickUser is the JSON request
type ReqKickUser struct {
	Reason string `json:"reason,omitempty"`
	UserID string `json:"user_id"`
}

// ReqBanUser is the JSON request
type ReqBanUser struct {
	Reason string `json:"reason,omitempty"`
	UserID string `json:"user_id"`
}

// ReqUnbanUser is the JSON request
type ReqUnbanUser struct {
	UserID string `json:"user_id"`
}

// ReqTyping is the JSON request
type ReqTyping struct {
	Typing  bool  `json:"typing"`
	Timeout int64 `json:"timeout"`
}
