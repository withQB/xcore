package xcore

import (
	"encoding/json"
	"fmt"
	"runtime/debug"
	"time"
)

// Syncer represents an interface that must be satisfied in order to do /sync requests on a client.
type Syncer interface {
	// Process the /sync response. The since parameter is the since= value that was used to produce the response.
	// This is useful for detecting the very first sync (since=""). If an error is return, Syncing will be stopped
	// permanently.
	ProcessResponse(resp *RespSync, since string) error
	// OnFailedSync returns either the time to wait before retrying or an error to stop syncing permanently.
	OnFailedSync(res *RespSync, err error) (time.Duration, error)
	// GetFilterJSON for the given user ID. NOT the filter ID.
	GetFilterJSON(userID string) json.RawMessage
}

// DefaultSyncer is the default syncing implementation. You can either write your own syncer, or selectively
// replace parts of this default syncer (e.g. the ProcessResponse method). The default syncer uses the observer
// pattern to notify callers about incoming events. See DefaultSyncer.OnEventType for more information.
type DefaultSyncer struct {
	UserID    string
	Store     Storer
	listeners map[string][]OnEventListener // event type to listeners array
}

// OnEventListener can be used with DefaultSyncer.OnEventType to be informed of incoming events.
type OnEventListener func(*Event)

// NewDefaultSyncer returns an instantiated DefaultSyncer
func NewDefaultSyncer(userID string, store Storer) *DefaultSyncer {
	return &DefaultSyncer{
		UserID:    userID,
		Store:     store,
		listeners: make(map[string][]OnEventListener),
	}
}

// ProcessResponse processes the /sync response in a way suitable for bots. "Suitable for bots" means a stream of
// unrepeating events. Returns a fatal error if a listener panics.
func (s *DefaultSyncer) ProcessResponse(res *RespSync, since string) (err error) {
	if !s.shouldProcessResponse(res, since) {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("ProcessResponse panicked! userID=%s since=%s panic=%s\n%s", s.UserID, since, r, debug.Stack())
		}
	}()

	for frameID, frameData := range res.Frames.Join {
		frame := s.getOrCreateFrame(frameID)
		for _, event := range frameData.State.Events {
			event.FrameID = frameID
			frame.UpdateState(&event)
			s.notifyListeners(&event)
		}
		for _, event := range frameData.Timeline.Events {
			event.FrameID = frameID
			s.notifyListeners(&event)
		}
		for _, event := range frameData.Ephemeral.Events {
			event.FrameID = frameID
			s.notifyListeners(&event)
		}
	}
	for frameID, frameData := range res.Frames.Invite {
		frame := s.getOrCreateFrame(frameID)
		for _, event := range frameData.State.Events {
			event.FrameID = frameID
			frame.UpdateState(&event)
			s.notifyListeners(&event)
		}
	}
	for frameID, frameData := range res.Frames.Leave {
		frame := s.getOrCreateFrame(frameID)
		for _, event := range frameData.Timeline.Events {
			if event.StateKey != nil {
				event.FrameID = frameID
				frame.UpdateState(&event)
				s.notifyListeners(&event)
			}
		}
	}
	return
}

// OnEventType allows callers to be notified when there are new events for the given event type.
// There are no duplicate checks.
func (s *DefaultSyncer) OnEventType(eventType string, callback OnEventListener) {
	_, exists := s.listeners[eventType]
	if !exists {
		s.listeners[eventType] = []OnEventListener{}
	}
	s.listeners[eventType] = append(s.listeners[eventType], callback)
}

// shouldProcessResponse returns true if the response should be processed. May modify the response to remove
// stuff that shouldn't be processed.
func (s *DefaultSyncer) shouldProcessResponse(resp *RespSync, since string) bool {
	if since == "" {
		return false
	}
	// This is a horrible hack because /sync will return the most recent messages for a frame
	// as soon as you /join it. We do NOT want to process those events in that particular frame
	// because they may have already been processed (if you toggle the bot in/out of the frame).
	//
	// Work around this by inspecting each frame's timeline and seeing if an m.frame.member event for us
	// exists and is "join" and then discard processing that frame entirely if so.
	// TDO: We probably want to process messages from after the last join event in the timeline.
	for frameID, frameData := range resp.Frames.Join {
		for i := len(frameData.Timeline.Events) - 1; i >= 0; i-- {
			e := frameData.Timeline.Events[i]
			if e.Type == "m.frame.member" && e.StateKey != nil && *e.StateKey == s.UserID {
				m := e.Content["membership"]
				mship, ok := m.(string)
				if !ok {
					continue
				}
				if mship == "join" {
					_, ok := resp.Frames.Join[frameID]
					if !ok {
						continue
					}
					delete(resp.Frames.Join, frameID)   // don't re-process messages
					delete(resp.Frames.Invite, frameID) // don't re-process invites
					break
				}
			}
		}
	}
	return true
}

// getOrCreateFrame must only be called by the Sync() goroutine which calls ProcessResponse()
func (s *DefaultSyncer) getOrCreateFrame(frameID string) *Frame {
	frame := s.Store.LoadFrame(frameID)
	if frame == nil { // create a new Frame
		frame = NewFrame(frameID)
		s.Store.SaveFrame(frame)
	}
	return frame
}

func (s *DefaultSyncer) notifyListeners(event *Event) {
	listeners, exists := s.listeners[event.Type]
	if !exists {
		return
	}
	for _, fn := range listeners {
		fn(event)
	}
}

// OnFailedSync always returns a 10 second wait period between failed /syncs, never a fatal error.
func (s *DefaultSyncer) OnFailedSync(res *RespSync, err error) (time.Duration, error) {
	return 10 * time.Second, nil
}

// GetFilterJSON returns a filter with a timeline limit of 50.
func (s *DefaultSyncer) GetFilterJSON(userID string) json.RawMessage {
	return json.RawMessage(`{"frame":{"timeline":{"limit":50}}}`)
}
