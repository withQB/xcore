package xcore

// RespError is the standard JSON error response. It also implements the Golang "error" interface.

type RespError struct {
	ErrCode string `json:"errcode"`
	Err     string `json:"error"`
}

// Error returns the errcode and error message.
func (e RespError) Error() string {
	return e.ErrCode + ": " + e.Err
}

// RespCreateFilter is the JSON response
type RespCreateFilter struct {
	FilterID string `json:"filter_id"`
}

// RespVersions is the JSON response
type RespVersions struct {
	Versions []string `json:"versions"`
}

// RespPublicFrames is the JSON response
type RespPublicFrames struct {
	TotalFrameCountEstimate int          `json:"total_frame_count_estimate"`
	PrevBatch              string       `json:"prev_batch"`
	NextBatch              string       `json:"next_batch"`
	Chunk                  []PublicFrame `json:"chunk"`
}

// RespJoinFrame is the JSON response
type RespJoinFrame struct {
	FrameID string `json:"frame_id"`
}

// RespLeaveFrame is the JSON response
type RespLeaveFrame struct{}

// RespForgetFrame is the JSON response 
type RespForgetFrame struct{}

// RespInviteUser is the JSON response
type RespInviteUser struct{}

// RespKickUser is the JSON response
type RespKickUser struct{}

// RespBanUser is the JSON response
type RespBanUser struct{}

// RespUnbanUser is the JSON response
type RespUnbanUser struct{}

// RespTyping is the JSON response
type RespTyping struct{}

// RespJoinedFrames is the JSON response
type RespJoinedFrames struct {
	JoinedFrames []string `json:"joined_frames"`
}

// RespJoinedMembers is the JSON response
type RespJoinedMembers struct {
	Joined map[string]struct {
		DisplayName *string `json:"display_name"`
		AvatarURL   *string `json:"avatar_url"`
	} `json:"joined"`
}

// RespMessages is the JSON response
type RespMessages struct {
	Start string  `json:"start"`
	Chunk []Event `json:"chunk"`
	End   string  `json:"end"`
}

// RespSendEvent is the JSON response
type RespSendEvent struct {
	EventID string `json:"event_id"`
}

// RespMediaUpload is the JSON response
type RespMediaUpload struct {
	ContentURI string `json:"content_uri"`
}

// RespUserInteractive is the JSON response
type RespUserInteractive struct {
	Flows []struct {
		Stages []string `json:"stages"`
	} `json:"flows"`
	Params    map[string]interface{} `json:"params"`
	Session   string                 `json:"session"`
	Completed []string               `json:"completed"`
	ErrCode   string                 `json:"errcode"`
	Error     string                 `json:"error"`
}

// HasSingleStageFlow returns true if there exists at least 1 Flow with a single stage of stageName.
func (r RespUserInteractive) HasSingleStageFlow(stageName string) bool {
	for _, f := range r.Flows {
		if len(f.Stages) == 1 && f.Stages[0] == stageName {
			return true
		}
	}
	return false
}

// RespUserDisplayName is the JSON response
type RespUserDisplayName struct {
	DisplayName string `json:"displayname"`
}

// RespUserStatus is the JSON response
type RespUserStatus struct {
	Presence        string `json:"presence"`
	StatusMsg       string `json:"status_msg"`
	LastActiveAgo   int    `json:"last_active_ago"`
	CurrentlyActive bool   `json:"currently_active"`
}

// RespRegister is the JSON response
type RespRegister struct {
	AccessToken  string `json:"access_token"`
	DeviceID     string `json:"device_id"`
	HomeServer   string `json:"home_server"`
	RefreshToken string `json:"refresh_token"`
	UserID       string `json:"user_id"`
}

// RespLogin is the JSON response
type RespLogin struct {
	AccessToken string               `json:"access_token"`
	DeviceID    string               `json:"device_id"`
	HomeServer  string               `json:"home_server"`
	UserID      string               `json:"user_id"`
	WellKnown   DiscoveryInformation `json:"well_known"`
}

// DiscoveryInformation is the JSON Response for get-well-known-coddy-client and a part of the JSON Response for post-coddy-client-r0-login
type DiscoveryInformation struct {
	Homeserver struct {
		BaseURL string `json:"base_url"`
	} `json:"m.homeserver"`
	IdentityServer struct {
		BaseURL string `json:"base_url"`
	} `json:"m.identitiy_server"`
}

// RespLogout is the JSON response
type RespLogout struct{}

// RespLogoutAll is the JSON response
type RespLogoutAll struct{}

// RespCreateFrame is the JSON response
type RespCreateFrame struct {
	FrameID string `json:"frame_id"`
}

// RespSync is the JSON response
type RespSync struct {
	NextBatch   string `json:"next_batch"`
	AccountData struct {
		Events []Event `json:"events"`
	} `json:"account_data"`
	Presence struct {
		Events []Event `json:"events"`
	} `json:"presence"`
	Frames struct {
		Leave map[string]struct {
			State struct {
				Events []Event `json:"events"`
			} `json:"state"`
			Timeline struct {
				Events    []Event `json:"events"`
				Limited   bool    `json:"limited"`
				PrevBatch string  `json:"prev_batch"`
			} `json:"timeline"`
		} `json:"leave"`
		Join map[string]struct {
			State struct {
				Events []Event `json:"events"`
			} `json:"state"`
			Timeline struct {
				Events    []Event `json:"events"`
				Limited   bool    `json:"limited"`
				PrevBatch string  `json:"prev_batch"`
			} `json:"timeline"`
			Ephemeral struct {
				Events []Event `json:"events"`
			} `json:"ephemeral"`
		} `json:"join"`
		Invite map[string]struct {
			State struct {
				Events []Event
			} `json:"invite_state"`
		} `json:"invite"`
	} `json:"frames"`
}

// RespTurnServer is the JSON response from a Turn Server
type RespTurnServer struct {
	Username string   `json:"username"`
	Password string   `json:"password"`
	TTL      int      `json:"ttl"`
	URIs     []string `json:"uris"`
}
