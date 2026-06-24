package api_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DirektorBani/datasafe/internal/auth"
)

func TestS3BucketLifecycleXML(t *testing.T) {
	s := testServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	creds := auth.Credentials{AccessKey: "datasafe", SecretKey: "datasafesecret"}

	mb, _ := http.NewRequest(http.MethodPut, ts.URL+"/lcxml", nil)
	_ = auth.SignRequest(mb, creds, "us-east-1", "s3", "UNSIGNED-PAYLOAD")
	resp, err := http.DefaultClient.Do(mb)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create bucket %d", resp.StatusCode)
	}

	putBody := `<LifecycleConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Rule>
    <ID>expire-logs</ID>
    <Prefix>logs/</Prefix>
    <Status>Enabled</Status>
    <Expiration><Days>30</Days></Expiration>
  </Rule>
</LifecycleConfiguration>`
	putReq, _ := http.NewRequest(http.MethodPut, ts.URL+"/lcxml?lifecycle", strings.NewReader(putBody))
	putReq.Header.Set("Content-Type", "application/xml")
	if err := auth.SignRequest(putReq, creds, "us-east-1", "s3", "UNSIGNED-PAYLOAD"); err != nil {
		t.Fatal(err)
	}
	putResp, err := http.DefaultClient.Do(putReq)
	if err != nil {
		t.Fatal(err)
	}
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("put lifecycle %d", putResp.StatusCode)
	}

	getReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/lcxml?lifecycle", nil)
	if err := auth.SignRequest(getReq, creds, "us-east-1", "s3", "UNSIGNED-PAYLOAD"); err != nil {
		t.Fatal(err)
	}
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(getResp.Body)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get lifecycle %d", getResp.StatusCode)
	}
	out := buf.String()
	if !strings.Contains(out, "expire-logs") || !strings.Contains(out, "<Days>30</Days>") {
		t.Fatalf("unexpected lifecycle xml: %s", out)
	}
}
