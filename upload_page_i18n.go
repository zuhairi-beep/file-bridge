package main

import (
	"encoding/json"
	"html/template"
	"regexp"
	"sort"
)

// uploadPageJS is embedded in the mobile upload page as var T = {...}.
// JSON keys must be camelCase and match every T.<key> reference in uploadPageTmpl.
type uploadPageJS struct {
	Uploading       string `json:"uploading"`
	UploadingPct    string `json:"uploadingPct"`
	SuccessSuffix   string `json:"successSuffix"`
	UploadFailed    string `json:"uploadFailed"`
	NetworkError    string `json:"networkError"`
	Cancelled       string `json:"cancelled"`
	Verifying       string `json:"verifying"`
	Verified        string `json:"verified"`
	Retrying        string `json:"retrying"`
	VerifiedPrefix  string `json:"verifiedPrefix"`
	VerifiedSuffix  string `json:"verifiedSuffix"`
	SafeToDelete    string `json:"safeToDelete"`
	PartialVerified string `json:"partialVerified"`
	FailedRetry     string `json:"failedRetry"`
}

func uploadTextsToJS(t uploadTexts) uploadPageJS {
	return uploadPageJS{
		Uploading:       t.Uploading,
		UploadingPct:    t.UploadingPct,
		SuccessSuffix:   t.SuccessSuffix,
		UploadFailed:    t.UploadFailed,
		NetworkError:    t.NetworkError,
		Cancelled:       t.Cancelled,
		Verifying:       t.Verifying,
		Verified:        t.Verified,
		Retrying:        t.Retrying,
		VerifiedPrefix:  t.VerifiedPrefix,
		VerifiedSuffix:  t.VerifiedSuffix,
		SafeToDelete:    t.SafeToDelete,
		PartialVerified: t.PartialVerified,
		FailedRetry:     t.FailedRetry,
	}
}

func marshalUploadPageJS(t uploadTexts) (template.JS, error) {
	b, err := json.Marshal(uploadTextsToJS(t))
	if err != nil {
		return "", err
	}
	return template.JS(b), nil
}

// uploadPageScriptI18nKeys lists T.<key> used in uploadPageTmpl (maintained by test).
var uploadPageScriptI18nKeys = []string{
	"uploading",
	"uploadFailed",
	"networkError",
	"cancelled",
	"verifying",
	"verified",
	"retrying",
	"verifiedPrefix",
	"verifiedSuffix",
	"safeToDelete",
	"partialVerified",
	"failedRetry",
}

var uploadPageTKeyRe = regexp.MustCompile(`T\.([a-zA-Z]+)`)

func keysUsedInUploadPageScript() []string {
	found := make(map[string]bool)
	for _, m := range uploadPageTKeyRe.FindAllStringSubmatch(uploadPageTmpl, -1) {
		if len(m) > 1 {
			found[m[1]] = true
		}
	}
	out := make([]string, 0, len(found))
	for k := range found {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func uploadPageJSMap(t uploadTexts) map[string]string {
	b, _ := json.Marshal(uploadTextsToJS(t))
	var m map[string]string
	json.Unmarshal(b, &m)
	return m
}
