package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type rpcRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func serve(r io.Reader, w io.Writer, app *App) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 32<<20)
	enc := json.NewEncoder(w)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = enc.Encode(rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}})
			continue
		}
		result, rpcErr := app.dispatch(req.Method, req.Params)
		app.noteRPC(req.Method, rpcErr)
		resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}
		if rpcErr != nil {
			resp.Error = rpcErr
		} else {
			resp.Result = result
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func (a *App) dispatch(method string, params map[string]any) (any, *rpcError) {
	if params == nil {
		params = map[string]any{}
	}
	switch method {
	case "profile.list":
		items, err := a.listProfiles()
		if err != nil {
			return nil, rpcFail(err)
		}
		return map[string]any{"profiles": items}, nil
	case "profile.put":
		profile, err := profileFrom(params["profile"])
		if err != nil {
			return nil, rpcFail(err)
		}
		saved, err := a.putProfile(profile)
		if err != nil {
			return nil, rpcFail(err)
		}
		return map[string]any{"profile": saved}, nil
	case "profile.delete":
		id, _ := params["id"].(string)
		if err := a.deleteProfile(id); err != nil {
			return nil, rpcFail(err)
		}
		return map[string]any{"deleted": id}, nil
	case "server.info":
		profile, err := a.profileFromParams(params)
		if err != nil {
			return nil, rpcFail(err)
		}
		token, _ := params["token"].(string)
		card, err := a.inspect(profile, token)
		if err != nil {
			return nil, rpcFail(err)
		}
		return map[string]any{"server": card}, nil
	case "auth.login":
		profile, err := a.profileFromParams(params)
		if err != nil {
			return nil, rpcFail(err)
		}
		username, _ := params["username"].(string)
		password, _ := params["password"].(string)
		result, err := a.loginPassword(profile, username, password)
		if err != nil {
			return nil, rpcFail(err)
		}
		return result, nil
	case "auth.mfa":
		profile, err := a.profileFromParams(params)
		if err != nil {
			return nil, rpcFail(err)
		}
		mfaToken, _ := params["mfa_token"].(string)
		code, _ := params["mfa_code"].(string)
		result, err := a.loginMFA(profile, mfaToken, code)
		if err != nil {
			return nil, rpcFail(err)
		}
		return result, nil
	case "auth.oidc":
		profile, err := a.profileFromParams(params)
		if err != nil {
			return nil, rpcFail(err)
		}
		username, _ := params["username"].(string)
		password, _ := params["password"].(string)
		result, err := a.loginOIDCPassword(profile, username, password)
		if err != nil {
			return nil, rpcFail(err)
		}
		return result, nil
	case "auth.oidc_exchange":
		profile, err := a.profileFromParams(params)
		if err != nil {
			return nil, rpcFail(err)
		}
		code, _ := params["exchange_code"].(string)
		result, err := a.loginOIDCExchange(profile, code)
		if err != nil {
			return nil, rpcFail(err)
		}
		return result, nil
	case "auth.token":
		profile, err := a.profileFromParams(params)
		if err != nil {
			return nil, rpcFail(err)
		}
		token, _ := params["token"].(string)
		expires, _ := params["expires_in"].(float64)
		result, err := a.acceptToken(profile, token, int(expires))
		if err != nil {
			return nil, rpcFail(err)
		}
		return result, nil
	case "auth.logout":
		profile, err := a.profileFromParams(params)
		if err != nil {
			return nil, rpcFail(err)
		}
		token, _ := params["token"].(string)
		result, err := a.logout(profile, token)
		if err != nil {
			return nil, rpcFail(err)
		}
		return result, nil
	case "session.probe":
		profile, err := a.profileFromParams(params)
		if err != nil {
			return nil, rpcFail(err)
		}
		token, _ := params["token"].(string)
		retry, _ := params["retry"].(bool)
		result, err := a.probe(profile, token, retry)
		if err != nil {
			return nil, rpcFail(err)
		}
		return result, nil
	case "account.me":
		return rpcBrowse(a.accountMe(params))
	case "bucket.list":
		return rpcBrowse(a.bucketList(params))
	case "bucket.create":
		return rpcBrowse(a.bucketCreate(params))
	case "bucket.delete":
		return rpcBrowse(a.bucketDelete(params))
	case "bucket.settings":
		return rpcBrowse(a.bucketSettings(params))
	case "bucket.encryption":
		return rpcBrowse(a.bucketEncryption(params))
	case "object.list":
		return rpcBrowse(a.objectList(params))
	case "object.meta":
		return rpcBrowse(a.objectMeta(params, http.MethodGet, nil))
	case "object.meta.put":
		return rpcBrowse(a.objectMeta(params, http.MethodPut, map[string]any{
			"metadata":     params["metadata"],
			"content_type": strParam(params, "content_type"),
		}))
	case "object.tags":
		return rpcBrowse(a.objectTags(params, http.MethodGet, nil))
	case "object.tags.put":
		return rpcBrowse(a.objectTags(params, http.MethodPut, map[string]any{"tags": params["tags"]}))
	case "object.retention":
		return rpcBrowse(a.objectRetention(params))
	case "object.versions":
		return rpcBrowse(a.objectVersions(params))
	case "object.restore":
		return rpcBrowse(a.objectAction(params, "restore", map[string]any{"version_id": strParam(params, "version_id")}))
	case "object.rename":
		return rpcBrowse(a.objectAction(params, "rename", map[string]any{"dest_key": strParam(params, "dest_key")}))
	case "object.delete":
		return rpcBrowse(a.objectDelete(params))
	case "folder.create":
		return rpcBrowse(a.folderCreate(params))
	case "folder.delete":
		return rpcBrowse(a.folderDelete(params))
	case "trash.list":
		return rpcBrowse(a.trashList(params))
	case "trash.restore":
		return rpcBrowse(a.trashRestore(params))
	case "trash.purge":
		return rpcBrowse(a.trashPurge(params))
	case "transfer.list":
		return rpcCall(a.transferList(params))
	case "transfer.enqueue":
		return rpcCall(a.transferEnqueue(params))
	case "transfer.cancel":
		return rpcCall(a.transferCancel(params))
	case "transfer.resolve":
		return rpcCall(a.transferResolve(params))
	case "transfer.files":
		return rpcCall(a.transferFiles(params))
	case "transfer.resume":
		return rpcCall(a.transferResume(params))
	case "transfer.retry":
		return rpcCall(a.transferRetry(params))
	case "diag.preview":
		return rpcCall(a.diagPreview(params))
	case "diag.export":
		return rpcCall(a.diagExport(params))
	default:
		return nil, &rpcError{Code: -32601, Message: "method not found"}
	}
}

func (a *App) profileFromParams(params map[string]any) (Profile, error) {
	if raw, ok := params["profile"]; ok {
		return profileFrom(raw)
	}
	id, _ := params["id"].(string)
	if id == "" {
		return Profile{}, fmt.Errorf("profile id is required")
	}
	return a.profileByID(id)
}

func profileFrom(raw any) (Profile, error) {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return Profile{}, err
	}
	var profile Profile
	if err := json.Unmarshal(encoded, &profile); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func rpcCall(result any, err error) (any, *rpcError) {
	if err != nil {
		return nil, rpcFail(err)
	}
	return result, nil
}

func rpcBrowse(result apiResult, err error) (any, *rpcError) {
	if err != nil {
		return nil, rpcFail(err)
	}
	return result, nil
}

func rpcFail(err error) *rpcError {
	return &rpcError{Code: -32000, Message: err.Error()}
}

func (a *App) noteRPC(method string, rpcErr *rpcError) {
	if a == nil || a.log == nil || method == "" {
		return
	}
	if rpcErr != nil {
		a.log.Error(method + " " + rpcErr.Message)
		return
	}
	if method == "transfer.list" {
		a.log.Debug(method)
		return
	}
	a.log.Info(method)
}
