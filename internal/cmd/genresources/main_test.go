package main

import (
	"strings"
	"testing"
)

func TestGoName(t *testing.T) {
	cases := map[string]string{
		"exact-aspects":      "ExactAspects",
		"kp-render":          "KPRender",
		"ai":                 "AI",
		"sky-events":         "SkyEvents",
		"chart-svg":          "ChartSVG",
		"move_into_new-home": "MoveIntoNewHome",
	}
	for in, want := range cases {
		if got := goName(in); got != want {
			t.Errorf("goName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAssignNames(t *testing.T) {
	newOp := func(id, method, path string) *operation {
		o := &operation{id: id, method: method, path: path, version: 1}
		if m := versionedPath.FindStringSubmatch(path); m != nil {
			o.version = int(m[1][0] - '0')
			segs := splitPath(m[2])
			o.resource, o.rest = segs[0], segs[1:]
		} else {
			o.resource, o.rest = splitPath(path)[0], []string{"simple"}
		}
		return o
	}
	ops := []*operation{
		newOp("v1_vedic_chart", "POST", "/api/v1/vedic/chart"),
		newOp("v2_vedic_chart", "POST", "/api/v2/vedic/chart"),
		newOp("v2_vedic_kp_render", "POST", "/api/v2/vedic/kp/render"),
		newOp("v2_vedic_kp", "POST", "/api/v2/vedic/kp"),
		newOp("v1_western_progressions_secondary_exact_aspects", "POST", "/api/v1/western/progressions/secondary/exact-aspects"),
		newOp("v1_western_progressions_secondary", "POST", "/api/v1/western/progressions/secondary"),
		newOp("v1_horoscope_daily_sign", "GET", "/api/v1/horoscope/daily/sign"),
		newOp("v3_horoscope_daily_sign", "GET", "/api/v3/horoscope/daily/sign"),
		newOp("v1_natal_report_job_id", "GET", "/api/v1/natal/report/{job_id}"),
		newOp("v1_things_delete", "DELETE", "/api/v1/things/{id}"),
		newOp("v1_sky_events", "GET", "/api/v1/sky-events"),
		newOp("simple_natal", "POST", "/natal"),
	}
	if err := assignNames(ops); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"v1_vedic_chart":     "Vedic.ChartV1",
		"v2_vedic_chart":     "Vedic.Chart",
		"v2_vedic_kp_render": "Vedic.KPRender",
		"v2_vedic_kp":        "Vedic.KP",
		"v1_western_progressions_secondary_exact_aspects": "Western.Progressions.SecondaryExactAspects",
		"v1_western_progressions_secondary":               "Western.Progressions.Secondary",
		"v1_horoscope_daily_sign":                         "Horoscope.Daily.SignV1",
		"v3_horoscope_daily_sign":                         "Horoscope.Daily.Sign",
		"v1_natal_report_job_id":                          "Natal.Report",
		"v1_things_delete":                                "Things.Delete",
		"v1_sky_events":                                   "SkyEvents.List",
		"simple_natal":                                    "Natal.Simple",
	}
	for _, o := range ops {
		if got := strings.Join(o.accessor, "."); got != want[o.id] {
			t.Errorf("%s: %s, want %s", o.id, got, want[o.id])
		}
	}
}

func TestAssignNamesRejectsClashes(t *testing.T) {
	ops := []*operation{
		{id: "a", method: "POST", path: "/api/v1/x/y", version: 1, resource: "x", rest: []string{"y"}},
		{id: "b", method: "PUT", path: "/api/v1/x/y", version: 1, resource: "x", rest: []string{"y"}},
	}
	if err := assignNames(ops); err == nil {
		t.Fatal("expected a name clash")
	}
}

func TestIsJSON(t *testing.T) {
	for ct, want := range map[string]bool{
		"application/json":                true,
		"application/json; charset=utf-8": true,
		"application/problem+json":        true,
		"application/vnd.freeastro+json":  true,
		"image/svg+xml":                   false,
		"text/markdown":                   false,
	} {
		if isJSON(ct) != want {
			t.Errorf("isJSON(%q) = %v", ct, !want)
		}
	}
}
