package xcore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client represents a Coddy client.
type Client struct {
	HomeserverURL *url.URL     // The base homeserver URL
	Prefix        string       // The API prefix eg '/_coddy/client/r0'
	UserID        string       // The user ID of the client. Used for forming HTTP paths which use the client's user ID.
	AccessToken   string       // The access_token for the client.
	Client        *http.Client // The underlying HTTP client which will be used to make HTTP requests.
	Syncer        Syncer       // The thing which can process /sync responses
	Store         Storer       // The thing which can store frames/tokens/ids

	// The ?user_id= query parameter for application services. This must be set *prior* to calling a method. If this is empty,
	// no user_id parameter will be sent.
	AppServiceUserID string

	syncingMutex sync.Mutex // protects syncingID
	syncingID    uint32     // Identifies the current Sync. Only one Sync can be active at any given time.
}

// HTTPError An HTTP Error response, which may wrap an underlying native Go Error.
type HTTPError struct {
	Contents     []byte
	WrappedError error
	Message      string
	Code         int
}

func (e HTTPError) Error() string {
	var wrappedErrMsg string
	if e.WrappedError != nil {
		wrappedErrMsg = e.WrappedError.Error()
	}
	return fmt.Sprintf("contents=%v msg=%s code=%d wrapped=%s", e.Contents, e.Message, e.Code, wrappedErrMsg)
}

// BuildURL builds a URL with the Client's homeserver/prefix set already.
func (cli *Client) BuildURL(urlPath ...string) string {
	ps := append([]string{cli.Prefix}, urlPath...)
	return cli.BuildBaseURL(ps...)
}

// BuildBaseURL builds a URL with the Client's homeserver set already. You must
// supply the prefix in the path.
func (cli *Client) BuildBaseURL(urlPath ...string) string {
	// copy the URL. Purposefully ignore error as the input is from a valid URL already
	hsURL, _ := url.Parse(cli.HomeserverURL.String())
	parts := []string{hsURL.Path}
	parts = append(parts, urlPath...)
	hsURL.Path = path.Join(parts...)
	// Manually add the trailing slash back to the end of the path if it's explicitly needed
	if strings.HasSuffix(urlPath[len(urlPath)-1], "/") {
		hsURL.Path = hsURL.Path + "/"
	}
	query := hsURL.Query()
	if cli.AppServiceUserID != "" {
		query.Set("user_id", cli.AppServiceUserID)
	}
	hsURL.RawQuery = query.Encode()
	return hsURL.String()
}

// BuildURLWithQuery builds a URL with query parameters in addition to the Client's homeserver/prefix set already.
func (cli *Client) BuildURLWithQuery(urlPath []string, urlQuery map[string]string) string {
	u, _ := url.Parse(cli.BuildURL(urlPath...))
	q := u.Query()
	for k, v := range urlQuery {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// SetCredentials sets the user ID and access token on this client instance.
func (cli *Client) SetCredentials(userID, accessToken string) {
	cli.AccessToken = accessToken
	cli.UserID = userID
}

// ClearCredentials removes the user ID and access token on this client instance.
func (cli *Client) ClearCredentials() {
	cli.AccessToken = ""
	cli.UserID = ""
}

// Sync starts syncing with the provided Homeserver. If Sync() is called twice then the first sync will be stopped and the
// error will be nil.
//
// This function will block until a fatal /sync error occurs, so it should almost always be started as a new goroutine.
// Fatal sync errors can be caused by:
//   - The failure to create a filter.
//   - Client.Syncer.OnFailedSync returning an error in response to a failed sync.
//   - Client.Syncer.ProcessResponse returning an error.
//
// If you wish to continue retrying in spite of these fatal errors, call Sync() again.
func (cli *Client) Sync() error {
	// Mark the client as syncing.
	// We will keep syncing until the syncing state changes. Either because
	// Sync is called or StopSync is called.
	syncingID := cli.incrementSyncingID()
	nextBatch := cli.Store.LoadNextBatch(cli.UserID)
	filterID := cli.Store.LoadFilterID(cli.UserID)
	if filterID == "" {
		filterJSON := cli.Syncer.GetFilterJSON(cli.UserID)
		resFilter, err := cli.CreateFilter(filterJSON)
		if err != nil {
			return err
		}
		filterID = resFilter.FilterID
		cli.Store.SaveFilterID(cli.UserID, filterID)
	}

	for {
		resSync, err := cli.SyncRequest(30000, nextBatch, filterID, false, "")
		if err != nil {
			duration, err2 := cli.Syncer.OnFailedSync(resSync, err)
			if err2 != nil {
				return err2
			}
			time.Sleep(duration)
			continue
		}

		// Check that the syncing state hasn't changed
		// Either because we've stopped syncing or another sync has been started.
		// We discard the response from our sync.
		if cli.getSyncingID() != syncingID {
			return nil
		}

		// Save the token now *before* processing it. This means it's possible
		// to not process some events, but it means that we won't get constantly stuck processing
		// a malformed/buggy event which keeps making us panic.
		cli.Store.SaveNextBatch(cli.UserID, resSync.NextBatch)
		if err = cli.Syncer.ProcessResponse(resSync, nextBatch); err != nil {
			return err
		}

		nextBatch = resSync.NextBatch
	}
}

func (cli *Client) incrementSyncingID() uint32 {
	cli.syncingMutex.Lock()
	defer cli.syncingMutex.Unlock()
	cli.syncingID++
	return cli.syncingID
}

func (cli *Client) getSyncingID() uint32 {
	cli.syncingMutex.Lock()
	defer cli.syncingMutex.Unlock()
	return cli.syncingID
}

// StopSync stops the ongoing sync started by Sync.
func (cli *Client) StopSync() {
	// Advance the syncing state so that any running Syncs will terminate.
	cli.incrementSyncingID()
}

// MakeRequest makes a JSON HTTP request to the given URL.
// The response body will be stream decoded into an interface. This will automatically stop if the response
// body is nil.
//
// Returns an error if the response is not 2xx along with the HTTP body bytes if it got that far. This error is
// an HTTPError which includes the returned HTTP status code, byte contents of the response body and possibly a
// RespError as the WrappedError, if the HTTP body could be decoded as a RespError.
func (cli *Client) MakeRequest(method string, httpURL string, reqBody interface{}, resBody interface{}) error {
	var req *http.Request
	var err error
	if reqBody != nil {
		buf := new(bytes.Buffer)
		if err := json.NewEncoder(buf).Encode(reqBody); err != nil {
			return err
		}
		req, err = http.NewRequest(method, httpURL, buf)
	} else {
		req, err = http.NewRequest(method, httpURL, nil)
	}

	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	if cli.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+cli.AccessToken)
	}

	res, err := cli.Client.Do(req)
	if res != nil {
		defer res.Body.Close()
	}
	if err != nil {
		return err
	}
	if res.StatusCode/100 != 2 { // not 2xx
		contents, err := io.ReadAll(res.Body)
		if err != nil {
			return err
		}

		var wrap error
		var respErr RespError
		if _ = json.Unmarshal(contents, &respErr); respErr.ErrCode != "" {
			wrap = respErr
		}

		// If we failed to decode as RespError, don't just drop the HTTP body, include it in the
		// HTTP error instead (e.g proxy errors which return HTML).
		msg := "Failed to " + method + " JSON to " + req.URL.Path
		if wrap == nil {
			msg = msg + ": " + string(contents)
		}

		return HTTPError{
			Contents:     contents,
			Code:         res.StatusCode,
			Message:      msg,
			WrappedError: wrap,
		}
	}

	if resBody != nil && res.Body != nil {
		return json.NewDecoder(res.Body).Decode(&resBody)
	}

	return nil
}

// CreateFilter makes an HTTP request according to post-coddy-client-r0-user-userid-filter
func (cli *Client) CreateFilter(filter json.RawMessage) (resp *RespCreateFilter, err error) {
	urlPath := cli.BuildURL("user", cli.UserID, "filter")
	err = cli.MakeRequest("POST", urlPath, &filter, &resp)
	return
}

// SyncRequest makes an HTTP request according to get-coddy-client-r0-sync
func (cli *Client) SyncRequest(timeout int, since, filterID string, fullState bool, setPresence string) (resp *RespSync, err error) {
	query := map[string]string{
		"timeout": strconv.Itoa(timeout),
	}
	if since != "" {
		query["since"] = since
	}
	if filterID != "" {
		query["filter"] = filterID
	}
	if setPresence != "" {
		query["set_presence"] = setPresence
	}
	if fullState {
		query["full_state"] = "true"
	}
	urlPath := cli.BuildURLWithQuery([]string{"sync"}, query)
	err = cli.MakeRequest("GET", urlPath, nil, &resp)
	return
}

func (cli *Client) register(u string, req *ReqRegister) (resp *RespRegister, uiaResp *RespUserInteractive, err error) {
	err = cli.MakeRequest("POST", u, req, &resp)
	if err != nil {
		httpErr, ok := err.(HTTPError)
		if !ok { // network error
			return
		}
		if httpErr.Code == 401 {
			// body should be RespUserInteractive, if it isn't, fail with the error
			err = json.Unmarshal(httpErr.Contents, &uiaResp)
			return
		}
	}
	return
}

// Register makes an HTTP request according to post-coddy-client-r0-register
//
// Registers with kind=user. For kind=guest, see RegisterGuest.
func (cli *Client) Register(req *ReqRegister) (*RespRegister, *RespUserInteractive, error) {
	u := cli.BuildURL("register")
	return cli.register(u, req)
}

// RegisterGuest makes an HTTP request according to post-coddy-client-r0-register
// with kind=guest.
//
// For kind=user, see Register.
func (cli *Client) RegisterGuest(req *ReqRegister) (*RespRegister, *RespUserInteractive, error) {
	query := map[string]string{
		"kind": "guest",
	}
	u := cli.BuildURLWithQuery([]string{"register"}, query)
	return cli.register(u, req)
}

// RegisterDummy performs m.login.dummy registration according
//
// Only a username and password need to be provided on the ReqRegister struct. Most local/developer homeservers will allow registration
// this way. If the homeserver does not, an error is returned.
//
// This does not set credentials on the client instance. See SetCredentials() instead.
//
//		res, err := cli.RegisterDummy(&gocoddy.ReqRegister{
//			Username: "alice",
//			Password: "wonderland",
//		})
//	 if err != nil {
//			panic(err)
//		}
//		token := res.AccessToken
func (cli *Client) RegisterDummy(req *ReqRegister) (*RespRegister, error) {
	res, uia, err := cli.Register(req)
	if err != nil && uia == nil {
		return nil, err
	}
	if uia != nil && uia.HasSingleStageFlow("m.login.dummy") {
		req.Auth = struct {
			Type    string `json:"type"`
			Session string `json:"session,omitempty"`
		}{"m.login.dummy", uia.Session}
		res, _, err = cli.Register(req)
		if err != nil {
			return nil, err
		}
	}
	if res == nil {
		return nil, fmt.Errorf("registration failed: does this server support m.login.dummy?")
	}
	return res, nil
}

// Login a user to the homeserver according to post-coddy-client-r0-login
// This does not set credentials on this client instance. See SetCredentials() instead.
func (cli *Client) Login(req *ReqLogin) (resp *RespLogin, err error) {
	urlPath := cli.BuildURL("login")
	err = cli.MakeRequest("POST", urlPath, req, &resp)
	return
}

// Logout the current user
// This does not clear the credentials from the client instance. See ClearCredentials() instead.
func (cli *Client) Logout() (resp *RespLogout, err error) {
	urlPath := cli.BuildURL("logout")
	err = cli.MakeRequest("POST", urlPath, nil, &resp)
	return
}

// LogoutAll logs the current user out on all devices. See post-coddy-client-r0-logout-all
// This does not clear the credentials from the client instance. See ClearCredentails() instead.
func (cli *Client) LogoutAll() (resp *RespLogoutAll, err error) {
	urlPath := cli.BuildURL("logout/all")
	err = cli.MakeRequest("POST", urlPath, nil, &resp)
	return
}

// Versions returns the list of supported Coddy versions on this homeserver. See get-coddy-client-versions
func (cli *Client) Versions() (resp *RespVersions, err error) {
	urlPath := cli.BuildBaseURL("_coddy", "client", "versions")
	err = cli.MakeRequest("GET", urlPath, nil, &resp)
	return
}

// PublicFrames returns the list of public frames on target server. See get-coddy-client-unstable-publicframes
func (cli *Client) PublicFrames(limit int, since string, server string) (resp *RespPublicFrames, err error) {
	args := map[string]string{}

	if limit != 0 {
		args["limit"] = strconv.Itoa(limit)
	}
	if since != "" {
		args["since"] = since
	}
	if server != "" {
		args["server"] = server
	}

	urlPath := cli.BuildURLWithQuery([]string{"publicFrames"}, args)
	err = cli.MakeRequest("GET", urlPath, nil, &resp)
	return
}

// PublicFramesFiltered returns a subset of PublicFrames filtered server side.
// See post-coddy-client-unstable-publicframes
func (cli *Client) PublicFramesFiltered(limit int, since string, server string, filter string) (resp *RespPublicFrames, err error) {
	content := map[string]string{}

	if limit != 0 {
		content["limit"] = strconv.Itoa(limit)
	}
	if since != "" {
		content["since"] = since
	}
	if filter != "" {
		content["filter"] = filter
	}

	var urlPath string
	if server == "" {
		urlPath = cli.BuildURL("publicFrames")
	} else {
		urlPath = cli.BuildURLWithQuery([]string{"publicFrames"}, map[string]string{
			"server": server,
		})
	}

	err = cli.MakeRequest("POST", urlPath, content, &resp)
	return
}

// JoinFrame joins the client to a frame ID or alias. See post-coddy-client-r0-join-frameidoralias
//
// If serverName is specified, this will be added as a query param to instruct the homeserver to join via that server. If content is specified, it will
// be JSON encoded and used as the request body.
func (cli *Client) JoinFrame(frameIDorAlias, serverName string, content interface{}) (resp *RespJoinFrame, err error) {
	var urlPath string
	if serverName != "" {
		urlPath = cli.BuildURLWithQuery([]string{"join", frameIDorAlias}, map[string]string{
			"server_name": serverName,
		})
	} else {
		urlPath = cli.BuildURL("join", frameIDorAlias)
	}
	err = cli.MakeRequest("POST", urlPath, content, &resp)
	return
}

// GetDisplayName returns the display name of the user from the specified MXID. See get-coddy-client-r0-profile-userid-displayname
func (cli *Client) GetDisplayName(mxid string) (resp *RespUserDisplayName, err error) {
	urlPath := cli.BuildURL("profile", mxid, "displayname")
	err = cli.MakeRequest("GET", urlPath, nil, &resp)
	return
}

// GetOwnDisplayName returns the user's display name. See get-coddy-client-r0-profile-userid-displayname
func (cli *Client) GetOwnDisplayName() (resp *RespUserDisplayName, err error) {
	urlPath := cli.BuildURL("profile", cli.UserID, "displayname")
	err = cli.MakeRequest("GET", urlPath, nil, &resp)
	return
}

// SetDisplayName sets the user's profile display name. See put-coddy-client-r0-profile-userid-displayname
func (cli *Client) SetDisplayName(displayName string) (err error) {
	urlPath := cli.BuildURL("profile", cli.UserID, "displayname")
	s := struct {
		DisplayName string `json:"displayname"`
	}{displayName}
	err = cli.MakeRequest("PUT", urlPath, &s, nil)
	return
}

// GetAvatarURL gets the user's avatar URL. See get-coddy-client-r0-profile-userid-avatar-url
func (cli *Client) GetAvatarURL() (string, error) {
	urlPath := cli.BuildURL("profile", cli.UserID, "avatar_url")
	s := struct {
		AvatarURL string `json:"avatar_url"`
	}{}

	err := cli.MakeRequest("GET", urlPath, nil, &s)
	if err != nil {
		return "", err
	}

	return s.AvatarURL, nil
}

// SetAvatarURL sets the user's avatar URL. See put-coddy-client-r0-profile-userid-avatar-url
func (cli *Client) SetAvatarURL(url string) error {
	urlPath := cli.BuildURL("profile", cli.UserID, "avatar_url")
	s := struct {
		AvatarURL string `json:"avatar_url"`
	}{url}
	err := cli.MakeRequest("PUT", urlPath, &s, nil)
	if err != nil {
		return err
	}

	return nil
}

// GetStatus returns the status of the user from the specified MXID. See get-coddy-client-r0-presence-userid-status
func (cli *Client) GetStatus(mxid string) (resp *RespUserStatus, err error) {
	urlPath := cli.BuildURL("presence", mxid, "status")
	err = cli.MakeRequest("GET", urlPath, nil, &resp)
	return
}

// GetOwnStatus returns the user's status. See get-coddy-client-r0-presence-userid-status
func (cli *Client) GetOwnStatus() (resp *RespUserStatus, err error) {
	return cli.GetStatus(cli.UserID)
}

// SetStatus sets the user's status. See put-coddy-client-r0-presence-userid-status
func (cli *Client) SetStatus(presence, status string) (err error) {
	urlPath := cli.BuildURL("presence", cli.UserID, "status")
	s := struct {
		Presence  string `json:"presence"`
		StatusMsg string `json:"status_msg"`
	}{presence, status}
	err = cli.MakeRequest("PUT", urlPath, &s, nil)
	return
}

// SendMessageEvent sends a message event into a frame. See put-coddy-client-r0-frames-frameid-send-eventtype-txnid
// contentJSON should be a pointer to something that can be encoded as JSON using json.Marshal.
func (cli *Client) SendMessageEvent(frameID string, eventType string, contentJSON interface{}) (resp *RespSendEvent, err error) {
	txnID := txnID()
	urlPath := cli.BuildURL("frames", frameID, "send", eventType, txnID)
	err = cli.MakeRequest("PUT", urlPath, contentJSON, &resp)
	return
}

// SendStateEvent sends a state event into a frame. See put-coddy-client-r0-frames-frameid-state-eventtype-statekey
// contentJSON should be a pointer to something that can be encoded as JSON using json.Marshal.
func (cli *Client) SendStateEvent(frameID, eventType, stateKey string, contentJSON interface{}) (resp *RespSendEvent, err error) {
	urlPath := cli.BuildURL("frames", frameID, "state", eventType, stateKey)
	err = cli.MakeRequest("PUT", urlPath, contentJSON, &resp)
	return
}

// SendText sends an m.frame.message event into the given frame with a msgtype of m.text
// See m-text
func (cli *Client) SendText(frameID, text string) (*RespSendEvent, error) {
	return cli.SendMessageEvent(frameID, "m.frame.message",
		TextMessage{MsgType: "m.text", Body: text})
}

// SendFormattedText sends an m.frame.message event into the given frame with a msgtype of m.text, supports a subset of HTML for formatting.
// See m-text
func (cli *Client) SendFormattedText(frameID, text, formattedText string) (*RespSendEvent, error) {
	return cli.SendMessageEvent(frameID, "m.frame.message",
		TextMessage{MsgType: "m.text", Body: text, FormattedBody: formattedText, Format: "org.coddy.custom.html"})
}

// SendImage sends an m.frame.message event into the given frame with a msgtype of m.image
// See m-image
func (cli *Client) SendImage(frameID, body, url string) (*RespSendEvent, error) {
	return cli.SendMessageEvent(frameID, "m.frame.message",
		ImageMessage{
			MsgType: "m.image",
			Body:    body,
			URL:     url,
		})
}

// SendVideo sends an m.frame.message event into the given frame with a msgtype of m.video
// See m-video
func (cli *Client) SendVideo(frameID, body, url string) (*RespSendEvent, error) {
	return cli.SendMessageEvent(frameID, "m.frame.message",
		VideoMessage{
			MsgType: "m.video",
			Body:    body,
			URL:     url,
		})
}

// SendNotice sends an m.frame.message event into the given frame with a msgtype of m.notice
// See m-notice
func (cli *Client) SendNotice(frameID, text string) (*RespSendEvent, error) {
	return cli.SendMessageEvent(frameID, "m.frame.message",
		TextMessage{MsgType: "m.notice", Body: text})
}

// RedactEvent redacts the given event. See put-coddy-client-r0-frames-frameid-redact-eventid-txnid
func (cli *Client) RedactEvent(frameID, eventID string, req *ReqRedact) (resp *RespSendEvent, err error) {
	txnID := txnID()
	urlPath := cli.BuildURL("frames", frameID, "redact", eventID, txnID)
	err = cli.MakeRequest("PUT", urlPath, req, &resp)
	return
}

// MarkRead marks eventID in frameID as read, signifying the event, and all before it have been read. See post-coddy-client-r0-frames-frameid-receipt-receipttype-eventid
func (cli *Client) MarkRead(frameID, eventID string) error {
	urlPath := cli.BuildURL("frames", frameID, "receipt", "m.read", eventID)
	return cli.MakeRequest("POST", urlPath, nil, nil)
}

// CreateFrame creates a new Coddy frame. See post-coddy-client-r0-createframe
//
//	resp, err := cli.CreateFrame(&gocoddy.ReqCreateFrame{
//		Preset: "public_chat",
//	})
//	fmt.Println("Frame:", resp.FrameID)
func (cli *Client) CreateFrame(req *ReqCreateFrame) (resp *RespCreateFrame, err error) {
	urlPath := cli.BuildURL("createFrame")
	err = cli.MakeRequest("POST", urlPath, req, &resp)
	return
}

// LeaveFrame leaves the given frame. See post-coddy-client-r0-frames-frameid-leave
func (cli *Client) LeaveFrame(frameID string) (resp *RespLeaveFrame, err error) {
	u := cli.BuildURL("frames", frameID, "leave")
	err = cli.MakeRequest("POST", u, struct{}{}, &resp)
	return
}

// ForgetFrame forgets a frame entirely. See post-coddy-client-r0-frames-frameid-forget
func (cli *Client) ForgetFrame(frameID string) (resp *RespForgetFrame, err error) {
	u := cli.BuildURL("frames", frameID, "forget")
	err = cli.MakeRequest("POST", u, struct{}{}, &resp)
	return
}

// InviteUser invites a user to a frame. See post-coddy-client-r0-frames-frameid-invite
func (cli *Client) InviteUser(frameID string, req *ReqInviteUser) (resp *RespInviteUser, err error) {
	u := cli.BuildURL("frames", frameID, "invite")
	err = cli.MakeRequest("POST", u, req, &resp)
	return
}

// InviteUserByThirdParty invites a third-party identifier to a frame. See invite-by-third-party-id-endpoint
func (cli *Client) InviteUserByThirdParty(frameID string, req *ReqInvite3PID) (resp *RespInviteUser, err error) {
	u := cli.BuildURL("frames", frameID, "invite")
	err = cli.MakeRequest("POST", u, req, &resp)
	return
}

// KickUser kicks a user from a frame. See post-coddy-client-r0-frames-frameid-kick
func (cli *Client) KickUser(frameID string, req *ReqKickUser) (resp *RespKickUser, err error) {
	u := cli.BuildURL("frames", frameID, "kick")
	err = cli.MakeRequest("POST", u, req, &resp)
	return
}

// BanUser bans a user from a frame. See post-coddy-client-r0-frames-frameid-ban
func (cli *Client) BanUser(frameID string, req *ReqBanUser) (resp *RespBanUser, err error) {
	u := cli.BuildURL("frames", frameID, "ban")
	err = cli.MakeRequest("POST", u, req, &resp)
	return
}

// UnbanUser unbans a user from a frame. See post-coddy-client-r0-frames-frameid-unban
func (cli *Client) UnbanUser(frameID string, req *ReqUnbanUser) (resp *RespUnbanUser, err error) {
	u := cli.BuildURL("frames", frameID, "unban")
	err = cli.MakeRequest("POST", u, req, &resp)
	return
}

// UserTyping sets the typing status of the user. See put-coddy-client-r0-frames-frameid-typing-userid
func (cli *Client) UserTyping(frameID string, typing bool, timeout int64) (resp *RespTyping, err error) {
	req := ReqTyping{Typing: typing, Timeout: timeout}
	u := cli.BuildURL("frames", frameID, "typing", cli.UserID)
	err = cli.MakeRequest("PUT", u, req, &resp)
	return
}

// StateEvent gets a single state event in a frame. It will attempt to JSON unmarshal into the given "outContent" struct with
// the HTTP response body, or return an error.
// See get-coddy-client-r0-frames-frameid-state-eventtype-statekey
func (cli *Client) StateEvent(frameID, eventType, stateKey string, outContent interface{}) (err error) {
	u := cli.BuildURL("frames", frameID, "state", eventType, stateKey)
	err = cli.MakeRequest("GET", u, nil, outContent)
	return
}

// UploadLink uploads an HTTP URL and then returns an MXC URI.
func (cli *Client) UploadLink(link string) (*RespMediaUpload, error) {
	res, err := cli.Client.Get(link)
	if res != nil {
		defer res.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	return cli.UploadToContentRepo(res.Body, res.Header.Get("Content-Type"), res.ContentLength)
}

// UploadToContentRepo uploads the given bytes to the content repository and returns an MXC URI.
// See post-coddy-media-r0-upload
func (cli *Client) UploadToContentRepo(content io.Reader, contentType string, contentLength int64) (*RespMediaUpload, error) {
	req, err := http.NewRequest("POST", cli.BuildBaseURL("_coddy/media/r0/upload"), content)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+cli.AccessToken)

	req.ContentLength = contentLength

	res, err := cli.Client.Do(req)
	if res != nil {
		defer res.Body.Close()
	}

	if err != nil {
		return nil, err
	}

	if res.StatusCode != 200 {
		contents, err := io.ReadAll(res.Body)
		if err != nil {
			return nil, HTTPError{
				Message: "Upload request failed - Failed to read response body: " + err.Error(),
				Code:    res.StatusCode,
			}
		}
		return nil, HTTPError{
			Contents: contents,
			Message:  "Upload request failed: " + string(contents),
			Code:     res.StatusCode,
		}
	}

	var m RespMediaUpload
	if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
		return nil, err
	}

	return &m, nil
}

// JoinedMembers returns a map of joined frame members. See TODO-SPEC. https://github.com/coddy-org/synapse/pull/1680
//
// In general, usage of this API is discouraged in favour of /sync, as calling this API can race with incoming membership changes.
// This API is primarily designed for application services which may want to efficiently look up joined members in a frame.
func (cli *Client) JoinedMembers(frameID string) (resp *RespJoinedMembers, err error) {
	u := cli.BuildURL("frames", frameID, "joined_members")
	err = cli.MakeRequest("GET", u, nil, &resp)
	return
}

// JoinedFrames returns a list of frames which the client is joined to. See TODO-SPEC. https://github.com/coddy-org/synapse/pull/1680
//
// In general, usage of this API is discouraged in favour of /sync, as calling this API can race with incoming membership changes.
// This API is primarily designed for application services which may want to efficiently look up joined frames.
func (cli *Client) JoinedFrames() (resp *RespJoinedFrames, err error) {
	u := cli.BuildURL("joined_frames")
	err = cli.MakeRequest("GET", u, nil, &resp)
	return
}

// Messages returns a list of message and state events for a frame. It uses
// pagination query parameters to paginate history in the frame.
// See get-coddy-client-r0-frames-frameid-messages
func (cli *Client) Messages(frameID, from, to string, dir rune, limit int) (resp *RespMessages, err error) {
	query := map[string]string{
		"from": from,
		"dir":  string(dir),
	}
	if to != "" {
		query["to"] = to
	}
	if limit != 0 {
		query["limit"] = strconv.Itoa(limit)
	}

	urlPath := cli.BuildURLWithQuery([]string{"frames", frameID, "messages"}, query)
	err = cli.MakeRequest("GET", urlPath, nil, &resp)
	return
}

// TurnServer returns turn server details and credentials for the client to use when initiating calls.
// See get-coddy-client-r0-voip-turnserver
func (cli *Client) TurnServer() (resp *RespTurnServer, err error) {
	urlPath := cli.BuildURL("voip", "turnServer")
	err = cli.MakeRequest("GET", urlPath, nil, &resp)
	return
}

func txnID() string {
	return "go" + strconv.FormatInt(time.Now().UnixNano(), 10)
}

// NewClient creates a new Coddy Client ready for syncing
func NewClient(homeserverURL, userID, accessToken string) (*Client, error) {
	hsURL, err := url.Parse(homeserverURL)
	if err != nil {
		return nil, err
	}
	// By default, use an in-memory store which will never save filter ids / next batch tokens to disk.
	// The client will work with this storer: it just won't remember across restarts.
	// In practice, a database backend should be used.
	store := NewInMemoryStore()
	cli := Client{
		AccessToken:   accessToken,
		HomeserverURL: hsURL,
		UserID:        userID,
		Prefix:        "/_coddy/client/r0",
		Syncer:        NewDefaultSyncer(userID, store),
		Store:         store,
	}
	// By default, use the default HTTP client.
	cli.Client = http.DefaultClient

	return &cli, nil
}
