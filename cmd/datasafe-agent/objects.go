package main

import (
	"net/http"
	"net/url"
	"strings"
)

// apiResult is a browse response. The body is the server JSON with wrapped
// key material removed. A rejected or blocked profile never leaves the agent.
type apiResult struct {
	Status  int            `json:"status"`
	Code    string         `json:"code,omitempty"`
	Message string         `json:"message,omitempty"`
	StepUp  bool           `json:"step_up,omitempty"`
	Body    map[string]any `json:"body,omitempty"`
}

func (a *App) browse(profile Profile, token, method, path string, query url.Values, payload any) (apiResult, error) {
	if profile.Session == "rejected" || (profile.Server != nil && profile.Server.Rejected) {
		return apiResult{Code: "server.too_old", Message: "server is older than 1.5.0"}, nil
	}
	if profile.Session == "blocked" || (profile.Server != nil && profile.Server.Blocked) {
		return apiResult{Code: "client.blocked", Message: "desktop version is blocked by the server"}, nil
	}
	if strings.TrimSpace(token) == "" {
		return apiResult{Status: http.StatusUnauthorized, Code: "auth.unauthorized", Message: "auth.unauthorized"}, nil
	}
	client, err := clientFor(profile)
	if err != nil {
		return apiResult{}, err
	}
	client.Token = token
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	resp, err := client.do(method, path, payload)
	if err != nil {
		return apiResult{}, err
	}
	body, _ := decodeObject(resp.Body)
	body = dropWrapped(body)
	if resp.Status >= 400 {
		apiErr := classifyError(resp.Status, resp.Body, resp.Header)
		message := safeMessage(apiErr)
		if apiErr.StepUp {
			message = "2FA is required. Complete it in the web console."
		}
		return apiResult{
			Status:  resp.Status,
			Code:    apiErr.Code,
			Message: message,
			StepUp:  apiErr.StepUp,
			Body:    body,
		}, nil
	}
	return apiResult{Status: resp.Status, Body: body}, nil
}

func (a *App) browseCall(params map[string]any, method, path string, query url.Values, payload any) (apiResult, error) {
	profile, err := a.profileFromParams(params)
	if err != nil {
		return apiResult{}, err
	}
	token, _ := params["token"].(string)
	return a.browse(profile, token, method, path, query, payload)
}

func bucketPath(params map[string]any) (string, error) {
	name, _ := params["bucket"].(string)
	name = strings.TrimSpace(name)
	if name == "" || strings.Contains(name, "/") {
		return "", errBucketName
	}
	return "/api/v1/buckets/" + url.PathEscape(name), nil
}

var errBucketName = errString("bucket name is required")

type errString string

func (e errString) Error() string { return string(e) }

func (a *App) bucketList(params map[string]any) (apiResult, error) {
	query := url.Values{}
	if filter := strParam(params, "filter"); filter != "" {
		query.Set("filter", filter)
	}
	return a.browseCall(params, http.MethodGet, "/api/v1/buckets", query, nil)
}

func (a *App) bucketCreate(params map[string]any) (apiResult, error) {
	path, err := bucketPath(params)
	if err != nil {
		return apiResult{}, err
	}
	return a.browseCall(params, http.MethodPost, path, nil, map[string]any{})
}

func (a *App) bucketDelete(params map[string]any) (apiResult, error) {
	path, err := bucketPath(params)
	if err != nil {
		return apiResult{}, err
	}
	return a.browseCall(params, http.MethodDelete, path, nil, nil)
}

func (a *App) bucketSettings(params map[string]any) (apiResult, error) {
	path, err := bucketPath(params)
	if err != nil {
		return apiResult{}, err
	}
	return a.browseCall(params, http.MethodGet, path+"/settings", nil, nil)
}

func (a *App) bucketEncryption(params map[string]any) (apiResult, error) {
	path, err := bucketPath(params)
	if err != nil {
		return apiResult{}, err
	}
	return a.browseCall(params, http.MethodGet, path+"/encryption", nil, nil)
}

func (a *App) objectList(params map[string]any) (apiResult, error) {
	path, err := bucketPath(params)
	if err != nil {
		return apiResult{}, err
	}
	query := url.Values{}
	if prefix := strParam(params, "prefix"); prefix != "" {
		query.Set("prefix", prefix)
	}
	delimiter := "/"
	if _, ok := params["delimiter"]; ok {
		delimiter = strParam(params, "delimiter")
	}
	if delimiter != "" {
		query.Set("delimiter", delimiter)
	}
	if marker := strParam(params, "start_after"); marker != "" {
		query.Set("start_after", marker)
	}
	maxKeys := intParam(params, "max_keys")
	if maxKeys <= 0 {
		maxKeys = 100
	}
	if maxKeys > 1000 {
		maxKeys = 1000
	}
	query.Set("max_keys", itoa(maxKeys))
	return a.browseCall(params, http.MethodGet, path+"/objects", query, nil)
}

func (a *App) objectMeta(params map[string]any, method string, payload any) (apiResult, error) {
	path, err := bucketPath(params)
	if err != nil {
		return apiResult{}, err
	}
	query := url.Values{}
	key := strParam(params, "key")
	if key == "" {
		return apiResult{}, errString("object key is required")
	}
	query.Set("key", key)
	if version := strParam(params, "version_id"); version != "" {
		query.Set("versionId", version)
	}
	return a.browseCall(params, method, path+"/object-meta", query, payload)
}

func (a *App) objectTags(params map[string]any, method string, payload any) (apiResult, error) {
	path, err := bucketPath(params)
	if err != nil {
		return apiResult{}, err
	}
	query := url.Values{}
	key := strParam(params, "key")
	if key == "" {
		return apiResult{}, errString("object key is required")
	}
	query.Set("key", key)
	return a.browseCall(params, method, path+"/object-tags", query, payload)
}

func (a *App) objectRetention(params map[string]any) (apiResult, error) {
	path, err := bucketPath(params)
	if err != nil {
		return apiResult{}, err
	}
	query := url.Values{}
	key := strParam(params, "key")
	if key == "" {
		return apiResult{}, errString("object key is required")
	}
	query.Set("key", key)
	if version := strParam(params, "version_id"); version != "" {
		query.Set("version_id", version)
	}
	return a.browseCall(params, http.MethodGet, path+"/object-retention", query, nil)
}

func (a *App) objectVersions(params map[string]any) (apiResult, error) {
	path, err := bucketPath(params)
	if err != nil {
		return apiResult{}, err
	}
	query := url.Values{}
	if prefix := strParam(params, "prefix"); prefix != "" {
		query.Set("prefix", prefix)
	}
	return a.browseCall(params, http.MethodGet, path+"/versions", query, nil)
}

func (a *App) objectAction(params map[string]any, action string, extra map[string]any) (apiResult, error) {
	path, err := bucketPath(params)
	if err != nil {
		return apiResult{}, err
	}
	key := strParam(params, "key")
	if key == "" {
		return apiResult{}, errString("object key is required")
	}
	payload := map[string]any{"action": action, "key": key}
	for name, value := range extra {
		payload[name] = value
	}
	return a.browseCall(params, http.MethodPost, path+"/object-actions", nil, payload)
}

func (a *App) objectDelete(params map[string]any) (apiResult, error) {
	path, err := bucketPath(params)
	if err != nil {
		return apiResult{}, err
	}
	keys := stringList(params["keys"])
	if len(keys) == 0 {
		if key := strParam(params, "key"); key != "" {
			keys = []string{key}
		}
	}
	if len(keys) == 0 {
		return apiResult{}, errString("object key is required")
	}
	return a.browseCall(params, http.MethodPost, path+"/bulk-delete", nil, map[string]any{"keys": keys})
}

func (a *App) folderCreate(params map[string]any) (apiResult, error) {
	path, err := bucketPath(params)
	if err != nil {
		return apiResult{}, err
	}
	name := strParam(params, "name")
	if name == "" {
		return apiResult{}, errString("folder name is required")
	}
	return a.browseCall(params, http.MethodPost, path+"/folders", nil, map[string]any{"name": name})
}

func (a *App) folderDelete(params map[string]any) (apiResult, error) {
	path, err := bucketPath(params)
	if err != nil {
		return apiResult{}, err
	}
	prefix := strParam(params, "prefix")
	if prefix == "" {
		return apiResult{}, errString("folder prefix is required")
	}
	return a.browseCall(params, http.MethodDelete, path+"/folders", nil, map[string]any{
		"prefix":    prefix,
		"recursive": boolParam(params, "recursive"),
	})
}

func (a *App) trashList(params map[string]any) (apiResult, error) {
	query := url.Values{}
	if bucket := strParam(params, "bucket"); bucket != "" {
		query.Set("bucket", bucket)
	}
	return a.browseCall(params, http.MethodGet, "/api/v1/trash", query, nil)
}

func (a *App) trashRestore(params map[string]any) (apiResult, error) {
	id := strParam(params, "trash_id")
	if id == "" {
		return apiResult{}, errString("trash id is required")
	}
	return a.browseCall(params, http.MethodPost, "/api/v1/trash/"+url.PathEscape(id)+"/restore", nil, nil)
}

func (a *App) trashPurge(params map[string]any) (apiResult, error) {
	id := strParam(params, "trash_id")
	if id == "" {
		return apiResult{}, errString("trash id is required")
	}
	return a.browseCall(params, http.MethodDelete, "/api/v1/trash/"+url.PathEscape(id), nil, nil)
}

func strParam(params map[string]any, key string) string {
	value, _ := params[key].(string)
	return strings.TrimSpace(value)
}

func intParam(params map[string]any, key string) int {
	switch value := params[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}

func boolParam(params map[string]any, key string) bool {
	value, _ := params[key].(bool)
	return value
}

func stringList(raw any) []string {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if ok && strings.TrimSpace(text) != "" {
			out = append(out, text)
		}
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func dropWrapped(body map[string]any) map[string]any {
	if body == nil {
		return map[string]any{}
	}
	return dropWrappedValue(body).(map[string]any)
}

func dropWrappedValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			if strings.EqualFold(key, "wrapped_dek") {
				continue
			}
			out[key] = dropWrappedValue(child)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, child := range typed {
			out[i] = dropWrappedValue(child)
		}
		return out
	default:
		return value
	}
}
