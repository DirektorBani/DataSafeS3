package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/DirektorBani/datasafe/clients/desktop/contract"
)

func TestFeatureColumns(t *testing.T) {
	v15 := features150()
	v16 := features160()
	seen := map[string]bool{}
	for _, row := range contract.Matrix {
		seen[row.Name] = true
		if row.In150 {
			if v15[row.Name] != 1 {
				t.Fatalf("1.5.0 missing %s", row.Name)
			}
		} else if _, ok := v15[row.Name]; ok {
			t.Fatalf("1.5.0 should not include %s", row.Name)
		}
		if row.In160 {
			if v16[row.Name] != 1 {
				t.Fatalf("1.6.0 missing %s", row.Name)
			}
		} else if _, ok := v16[row.Name]; ok {
			t.Fatalf("1.6.0 should not include %s", row.Name)
		}
	}
	for key := range v15 {
		if !seen[key] {
			t.Fatalf("1.5.0 has %s, which is not in table 4.6.6", key)
		}
	}
	for key := range v16 {
		if !seen[key] {
			t.Fatalf("1.6.0 has %s, which is not in table 4.6.6", key)
		}
	}
	if _, ok := v16["errors.codes"]; ok {
		t.Fatal("errors.codes arrives only with the capabilities feature map")
	}
}

func TestDetectVersions(t *testing.T) {
	publicNoFeatures := &capsInfo{Reached: true, Status: 200, API: "v1", ServerVersion: "1.6.1", LoginMethods: []string{"password"}}
	cases := []struct {
		name    string
		public  *capsInfo
		authed  *capsInfo
		health  healthInfo
		me      bool
		profile string
		reject  bool
		block   bool
		warn    bool
		feature string
		absent  string
	}{
		{
			name:    "capabilities features win",
			authed:  &capsInfo{Reached: true, Status: 200, API: "v1", HasFeatures: true, Features: map[string]int{"auth.password": 1, "errors.codes": 1, "future.widget": 2}, ServerVersion: "1.6.1"},
			profile: "features",
			feature: "errors.codes",
		},
		{
			name:    "public body without features uses healthz 1.6.1",
			public:  publicNoFeatures,
			health:  healthInfo{Reached: true, Version: "1.6.1"},
			profile: "1.6.0",
			feature: "sharing.policy",
			absent:  "errors.codes",
		},
		{
			name:    "1.6.0",
			health:  healthInfo{Reached: true, Version: "1.6.0"},
			profile: "1.6.0",
			feature: "encryption.kms",
		},
		{
			name:    "1.5.0 stamped",
			health:  healthInfo{Reached: true, Version: "1.5.0"},
			profile: "1.5.0",
			feature: "sharing.links",
			absent:  "sharing.policy",
		},
		{
			name:    "1.5.0 dev without kms",
			health:  healthInfo{Reached: true, Version: "dev"},
			profile: "1.5.0",
			absent:  "auth.step_up",
		},
		{
			name:    "dev with kms is 1.6.0",
			health:  healthInfo{Reached: true, Version: "dev", HasKMS: true},
			profile: "1.6.0",
			feature: "auth.step_up",
		},
		{
			name:    "1.4.1 is rejected",
			health:  healthInfo{Reached: true, Version: "1.4.1"},
			profile: "rejected",
			reject:  true,
		},
		{
			name:    "reachable healthz with no version is older than 1.5.0",
			health:  healthInfo{Reached: true},
			profile: "rejected",
			reject:  true,
		},
		{
			name:    "reachable healthz with a blank version is older than 1.5.0",
			health:  healthInfo{Reached: true, Version: "  "},
			profile: "rejected",
			reject:  true,
		},
		{
			name:    "unknown version is basic",
			health:  healthInfo{Reached: true, Version: "banana"},
			me:      true,
			profile: "basic",
			warn:    true,
			feature: "objects.multipart",
		},
		{
			name:    "healthz down but me answers",
			me:      true,
			profile: "basic",
			warn:    true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			card := detect(tc.public, tc.authed, tc.health, tc.me, DesktopVersion, "")
			if card.Profile != tc.profile {
				t.Fatalf("profile %s, want %s (%+v)", card.Profile, tc.profile, card)
			}
			if card.Rejected != tc.reject {
				t.Fatalf("rejected=%v", card.Rejected)
			}
			if card.Blocked != tc.block {
				t.Fatalf("blocked=%v", card.Blocked)
			}
			if tc.warn && card.Warning == "" {
				t.Fatal("expected a warning")
			}
			if tc.feature != "" && card.Features[tc.feature] < 1 {
				t.Fatalf("missing feature %s in %#v", tc.feature, card.Features)
			}
			if tc.absent != "" {
				if _, ok := card.Features[tc.absent]; ok {
					t.Fatalf("feature %s should be absent", tc.absent)
				}
			}
			if tc.reject || tc.block {
				if card.BrowseAllowed {
					t.Fatal("browse should be blocked")
				}
			} else if !card.BrowseAllowed {
				t.Fatal("browse should stay available")
			}
		})
	}
}

func TestClientVersionPolicy(t *testing.T) {
	desktop := map[string]any{"min_supported": "1.2.0"}
	card := detect(nil, &capsInfo{
		Reached: true, Status: 200, API: "v1", HasFeatures: true,
		Features: map[string]int{"auth.password": 1},
		Desktop:  desktop,
	}, healthInfo{}, false, "1.0.0-dev", "")
	if card.Blocked || card.Warning == "" || !card.BrowseAllowed {
		t.Fatalf("min_supported should warn and keep working: %+v", card)
	}

	card = detect(nil, &capsInfo{
		Reached: true, Status: 200, API: "v1", HasFeatures: true,
		Features: map[string]int{"auth.password": 1},
		Desktop:  map[string]any{"min_supported": "1.0.0"},
	}, healthInfo{}, false, "1.0.0-dev", "")
	if card.Warning != "" || card.Blocked {
		t.Fatalf("dev build of 1.0.0 should satisfy min_supported 1.0.0: %+v", card)
	}

	card = detect(nil, &capsInfo{
		Reached: true, Status: 200, API: "v1", HasFeatures: true,
		Features: map[string]int{"auth.password": 1},
		Desktop: map[string]any{
			"min_required":        "9.0.0",
			"min_required_reason": "security",
			"advisory_url":        "https://example.test/advisory",
		},
	}, healthInfo{}, false, DesktopVersion, "")
	if !card.Blocked || card.BrowseAllowed || card.AdvisoryURL == "" {
		t.Fatalf("security floor should block: %+v", card)
	}

	card = detect(nil, &capsInfo{
		Reached: true, Status: 200, API: "v1", HasFeatures: true,
		Features: map[string]int{"auth.password": 1},
		Desktop:  map[string]any{"min_required": "9.0.0", "min_required_reason": "other"},
	}, healthInfo{}, false, DesktopVersion, "")
	if card.Blocked || card.Warning == "" {
		t.Fatalf("non-security floor is a warning: %+v", card)
	}
}

func TestParseCapsFixtures(t *testing.T) {
	subset := fixtureBody(t, "partial/capabilities-subset.json")
	parsed := parseCaps(200, subset)
	parsed.Reached = true
	if !parsed.HasFeatures || parsed.API != "v1" || parsed.Features["auth.password"] != 1 {
		t.Fatalf("partial features: %+v", parsed)
	}
	if _, ok := parsed.Features["sharing.links"]; ok {
		t.Fatal("a missing feature key is off")
	}
	card := detect(nil, parsed, healthInfo{Reached: true, Version: "1.5.0"}, false, DesktopVersion, "")
	if card.Profile != "features" || card.Features["future.widget"] != 2 || card.Features["sharing.policy"] != 0 {
		t.Fatalf("unknown keys stay inert and missing keys stay off: %+v", card)
	}

	plain := fixtureBody(t, "partial/capabilities-no-features.json")
	public := parseCaps(200, plain)
	public.Reached = true
	if public.HasFeatures || public.API != "v1" {
		t.Fatalf("body without features: %+v", public)
	}
	card = detect(public, nil, healthInfo{Reached: true, Version: "1.6.1"}, false, DesktopVersion, "")
	if card.Profile != "1.6.0" || card.Features["sharing.policy"] != 1 {
		t.Fatalf("no features falls through to healthz: %+v", card)
	}
	if _, ok := card.Features["errors.codes"]; ok {
		t.Fatal("healthz 1.6 must not invent errors.codes")
	}

	broken := fixtureRaw(t, "partial/capabilities-broken.json")
	parsed = parseCaps(200, broken)
	if parsed.HasFeatures || parsed.API != "" {
		t.Fatalf("broken JSON must not invent features: %+v", parsed)
	}
	card = detect(&capsInfo{Reached: true, Status: 200}, nil, healthInfo{Reached: true, Version: "1.5.0"}, false, DesktopVersion, "")
	if card.Profile != "1.5.0" || card.Features["sharing.links"] != 1 {
		t.Fatalf("unparsed capabilities use healthz: %+v", card)
	}
	if _, ok := card.Features["sharing.policy"]; ok {
		t.Fatal("1.5.0 column includes sharing.policy")
	}

	missing := fixtureBody(t, "1.5.0/capabilities.json")
	parsed = parseCaps(404, missing)
	if parsed.HasFeatures {
		t.Fatal("404 capabilities is not a feature map")
	}
	health := fixtureBody(t, "1.6.0/healthz.json")
	var healthObj map[string]any
	if err := json.Unmarshal(health, &healthObj); err != nil {
		t.Fatal(err)
	}
	version, _ := healthObj["version"].(string)
	card = detect(&capsInfo{Reached: true, Status: 404}, nil, healthInfo{Reached: true, Version: version}, false, DesktopVersion, "")
	if card.Profile != "1.6.0" || card.Features["encryption.kms"] != 1 {
		t.Fatalf("404 capabilities plus 1.6.0 healthz: %+v", card)
	}
}

func fixtureBody(t *testing.T, name string) []byte {
	t.Helper()
	var env struct {
		Body json.RawMessage `json:"body"`
	}
	raw := fixtureRaw(t, name)
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Body) == 0 {
		t.Fatalf("%s has no body", name)
	}
	return env.Body
}

func fixtureRaw(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "clients", "desktop", "contract", "fixtures", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestBelow(t *testing.T) {
	if below("1.0.0-dev", "1.0.0") {
		t.Fatal("same triple is not below")
	}
	if !below("1.0.0-dev", "1.2.0") {
		t.Fatal("1.0.0-dev is below 1.2.0")
	}
	if below("1.6.1", "1.6.0") {
		t.Fatal("newer is not below")
	}
}
