package xcore

// Frame represents a single Coddy frame.
type Frame struct {
	ID    string
	State map[string]map[string]*Event
}

// PublicFrame represents the information about a public frame obtainable from the frame directory
type PublicFrame struct {
	CanonicalAlias   string   `json:"canonical_alias"`
	Name             string   `json:"name"`
	WorldReadable    bool     `json:"world_readable"`
	Topic            string   `json:"topic"`
	NumJoinedMembers int      `json:"num_joined_members"`
	AvatarURL        string   `json:"avatar_url"`
	FrameID           string   `json:"frame_id"`
	GuestCanJoin     bool     `json:"guest_can_join"`
	Aliases          []string `json:"aliases"`
}

// UpdateState updates the frame's current state with the given Event. This will clobber events based
// on the type/state_key combination.
func (frame Frame) UpdateState(event *Event) {
	_, exists := frame.State[event.Type]
	if !exists {
		frame.State[event.Type] = make(map[string]*Event)
	}
	frame.State[event.Type][*event.StateKey] = event
}

// GetStateEvent returns the state event for the given type/state_key combo, or nil.
func (frame Frame) GetStateEvent(eventType string, stateKey string) *Event {
	stateEventMap := frame.State[eventType]
	event := stateEventMap[stateKey]
	return event
}

// GetMembershipState returns the membership state of the given user ID in this frame. If there is
// no entry for this member, 'leave' is returned for consistency with left users.
func (frame Frame) GetMembershipState(userID string) string {
	state := "leave"
	event := frame.GetStateEvent("m.frame.member", userID)
	if event != nil {
		membershipState, found := event.Content["membership"]
		if found {
			mState, isString := membershipState.(string)
			if isString {
				state = mState
			}
		}
	}
	return state
}

// NewFrame creates a new Frame with the given ID
func NewFrame(frameID string) *Frame {
	// Init the State map and return a pointer to the Frame
	return &Frame{
		ID:    frameID,
		State: make(map[string]map[string]*Event),
	}
}
