package xcore

import "errors"

// Filter is used by clients to specify how the server should filter responses to e.g. sync requests
// Specified by: https://matrix.org/docs/spec/client_server/r0.2.0.html#filtering
type Filter struct {
	AccountData FilterPart `json:"account_data,omitempty"`
	EventFields []string   `json:"event_fields,omitempty"`
	EventFormat string     `json:"event_format,omitempty"`
	Presence    FilterPart `json:"presence,omitempty"`
	Frame        FrameFilter `json:"frame,omitempty"`
}

// FrameFilter is used to define filtering rules for frame events
type FrameFilter struct {
	AccountData  FilterPart `json:"account_data,omitempty"`
	Ephemeral    FilterPart `json:"ephemeral,omitempty"`
	IncludeLeave bool       `json:"include_leave,omitempty"`
	NotFrames     []string   `json:"not_frames,omitempty"`
	Frames        []string   `json:"frames,omitempty"`
	State        FilterPart `json:"state,omitempty"`
	Timeline     FilterPart `json:"timeline,omitempty"`
}

// FilterPart is used to define filtering rules for specific categories of events
type FilterPart struct {
	NotFrames    []string `json:"not_frames,omitempty"`
	Frames       []string `json:"frames,omitempty"`
	Limit       int      `json:"limit,omitempty"`
	NotSenders  []string `json:"not_senders,omitempty"`
	NotTypes    []string `json:"not_types,omitempty"`
	Senders     []string `json:"senders,omitempty"`
	Types       []string `json:"types,omitempty"`
	ContainsURL *bool    `json:"contains_url,omitempty"`
}

// Validate checks if the filter contains valid property values
func (filter *Filter) Validate() error {
	if filter.EventFormat != "client" && filter.EventFormat != "federation" {
		return errors.New("bad event_format value. Must be one of [\"client\", \"federation\"]")
	}
	return nil
}

// DefaultFilter returns the default filter used by the Matrix server if no filter is provided in the request
func DefaultFilter() Filter {
	return Filter{
		AccountData: DefaultFilterPart(),
		EventFields: nil,
		EventFormat: "client",
		Presence:    DefaultFilterPart(),
		Frame: FrameFilter{
			AccountData:  DefaultFilterPart(),
			Ephemeral:    DefaultFilterPart(),
			IncludeLeave: false,
			NotFrames:     nil,
			Frames:        nil,
			State:        DefaultFilterPart(),
			Timeline:     DefaultFilterPart(),
		},
	}
}

// DefaultFilterPart returns the default filter part used by the Matrix server if no filter is provided in the request
func DefaultFilterPart() FilterPart {
	return FilterPart{
		NotFrames:   nil,
		Frames:      nil,
		Limit:      20,
		NotSenders: nil,
		NotTypes:   nil,
		Senders:    nil,
		Types:      nil,
	}
}
