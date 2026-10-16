package main

import "testing"

func TestFeatureColumns(t *testing.T) {
	v15 := features150()
	for _, key := range []string{"auth.password", "auth.totp", "auth.oidc", "objects.multipart", "sharing.links"} {
		if v15[key] != 1 {
			t.Fatalf("1.5.0 missing %s", key)
		}
	}
	for _, key := range []string{"sharing.policy", "sharing.revoke_all", "auth.step_up", "encryption.kms", "errors.codes", "auth.refresh"} {
		if _, ok := v15[key]; ok {
			t.Fatalf("1.5.0 should not include %s", key)
		}
	}
	v16 := features160()
	for _, key := range []string{"sharing.policy", "sharing.revoke_all", "auth.step_up", "encryption.kms"} {
		if v16[key] != 1 {
			t.Fatalf("1.6.0 missing %s", key)
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
			card := detect(tc.public, tc.authed, tc.health, tc.me, DesktopVersion)
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
	}, healthInfo{}, false, "1.0.0-dev")
	if card.Blocked || card.Warning == "" || !card.BrowseAllowed {
		t.Fatalf("min_supported should warn and keep working: %+v", card)
	}

	card = detect(nil, &capsInfo{
		Reached: true, Status: 200, API: "v1", HasFeatures: true,
		Features: map[string]int{"auth.password": 1},
		Desktop:  map[string]any{"min_supported": "1.0.0"},
	}, healthInfo{}, false, "1.0.0-dev")
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
	}, healthInfo{}, false, DesktopVersion)
	if !card.Blocked || card.BrowseAllowed || card.AdvisoryURL == "" {
		t.Fatalf("security floor should block: %+v", card)
	}

	card = detect(nil, &capsInfo{
		Reached: true, Status: 200, API: "v1", HasFeatures: true,
		Features: map[string]int{"auth.password": 1},
		Desktop:  map[string]any{"min_required": "9.0.0", "min_required_reason": "other"},
	}, healthInfo{}, false, DesktopVersion)
	if card.Blocked || card.Warning == "" {
		t.Fatalf("non-security floor is a warning: %+v", card)
	}
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
