package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

func ptr[T any](v T) *T { return &v }

func fullInput() Input {
	return Input{Battery: BatteryOverride{
		Name: ptr("test"), EnergyCapacityMWh: ptr(100.0), PowerCapacityMW: ptr(50.0),
		ChargeEfficiency: ptr(0.95), DischargeEfficiency: ptr(0.9), MinSOC: ptr(0.1), MaxSOC: ptr(0.9),
	}, Strategy: StrategyOverride{Name: ptr("schedule")}}
}

func requireField(t *testing.T, err error, field string) {
	t.Helper()
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Field != field {
		t.Fatalf("wanted field %q, got %v", field, err)
	}
}

func TestResolveDefaultsAndExplicitZero(t *testing.T) {
	in := fullInput()
	out, err := Resolve(in, PresetCatalog{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Battery.InitialSOC != .1 || out.Battery.DegradationCostPerMWh != 0 {
		t.Fatalf("battery defaults: %+v", out.Battery)
	}
	want := ScheduleConfig{ChargeStartMinute: 600, ChargeEndMinute: 1020, DischargeStartMinute: 1020, DischargeEndMinute: 1439, ChargePowerMW: 50, DischargePowerMW: 50}
	if out.Strategy != want {
		t.Fatalf("schedule defaults: %+v", out.Strategy)
	}
	in.Battery.MinSOC, in.Battery.InitialSOC, in.Battery.DegradationCostPerMWh = ptr(0.0), ptr(0.0), ptr(0.0)
	in.Strategy.Params.ChargePowerMW = ptr(0.0)
	out, err = Resolve(in, PresetCatalog{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Battery.InitialSOC != 0 || out.Strategy.(ScheduleConfig).ChargePowerMW != 0 {
		t.Fatalf("zero lost: %+v", out)
	}
	in.Strategy = StrategyOverride{Name: ptr("oracle")}
	out, err = Resolve(in, PresetCatalog{})
	if err != nil || out.Strategy != (OracleConfig{SOCSteps: 200, PowerSteps: 10}) {
		t.Fatalf("oracle defaults: %+v, %v", out, err)
	}
}

func TestBatteryRequiredAndPhysicalValidation(t *testing.T) {
	cases := []struct {
		field   string
		target  func(*BatteryOverride) **float64
		invalid float64
	}{
		{"energy_capacity_mwh", func(b *BatteryOverride) **float64 { return &b.EnergyCapacityMWh }, 0},
		{"power_capacity_mw", func(b *BatteryOverride) **float64 { return &b.PowerCapacityMW }, 0},
		{"charge_efficiency", func(b *BatteryOverride) **float64 { return &b.ChargeEfficiency }, 0},
		{"discharge_efficiency", func(b *BatteryOverride) **float64 { return &b.DischargeEfficiency }, 1.01},
		{"min_soc", func(b *BatteryOverride) **float64 { return &b.MinSOC }, -0.01},
		{"max_soc", func(b *BatteryOverride) **float64 { return &b.MaxSOC }, 0.05},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			in := fullInput()
			*tc.target(&in.Battery) = nil
			_, err := Resolve(in, PresetCatalog{})
			requireField(t, err, "battery."+tc.field)
			*tc.target(&in.Battery) = ptr(tc.invalid)
			_, err = Resolve(in, PresetCatalog{})
			requireField(t, err, "battery."+tc.field)
		})
	}
	for _, field := range []string{"InitialSOC", "DegradationCostPerMWh", "EnergyCapacityMWh", "PowerCapacityMW", "ChargeEfficiency", "DischargeEfficiency", "MinSOC", "MaxSOC"} {
		for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
			in := fullInput()
			reflect.ValueOf(&in.Battery).Elem().FieldByName(field).Set(reflect.ValueOf(ptr(bad)))
			if _, err := Resolve(in, PresetCatalog{}); err == nil {
				t.Fatalf("accepted %s=%v", field, bad)
			}
		}
	}
	for _, bad := range []float64{0, 1} {
		in := fullInput()
		in.Battery.InitialSOC = ptr(bad)
		_, err := Resolve(in, PresetCatalog{})
		requireField(t, err, "battery.initial_soc")
	}
	in := fullInput()
	in.Battery.DegradationCostPerMWh = ptr(-1.0)
	_, err := Resolve(in, PresetCatalog{})
	requireField(t, err, "battery.degradation_cost_per_mwh")
}

func TestStrictJSONAndYAML(t *testing.T) {
	fields := []string{"name", "energy_capacity_mwh", "power_capacity_mw", "charge_efficiency", "discharge_efficiency", "min_soc", "max_soc", "initial_soc", "degradation_cost_per_mwh"}
	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			for _, decode := range []func([]byte, any) error{json.Unmarshal, yaml.Unmarshal} {
				var input Input
				requireField(t, decode([]byte(fmt.Sprintf(`{"battery":{"%s":null}}`, field)), &input), "battery."+field)
				value := "0"
				if field == "name" {
					value = `""`
				}
				if err := decode([]byte(fmt.Sprintf(`{"battery":{"%s":%s}}`, field, value)), &input); err != nil {
					t.Fatal(err)
				}
				b := reflect.ValueOf(input.Battery)
				found := false
				for idx := 0; idx < b.NumField(); idx++ {
					if strings.Split(b.Type().Field(idx).Tag.Get("json"), ",")[0] == field {
						found = !b.Field(idx).IsNil()
					}
				}
				if !found {
					t.Fatalf("lost present %s", field)
				}
			}
		})
	}
	cases := []struct{ raw, field string }{
		{`{"battery_file":null}`, "battery_file"}, {`{"battery_file":""}`, "battery_file"},
		{`{"battery":null}`, "battery"}, {`{"strategy":null}`, "strategy"},
		{`{"strategy":{"name":null}}`, "strategy.name"}, {`{"strategy":{"name":""}}`, "strategy.name"},
		{`{"strategy":{"params":null}}`, "strategy.params"},
		{`{"battery":{"watts":1}}`, "battery.watts"}, {`{"unknown":1}`, "unknown"},
		{`{"battery":{"name":12}}`, "battery.name"}, {`{"battery":{"min_soc":"0"}}`, "battery.min_soc"},
		{`{"battery":{"min_soc":false}}`, "battery.min_soc"}, {`{"battery":[]}`, "battery"},
		{`{"battery":{"min_soc":0,"min_soc":1}}`, "battery.min_soc"},
		{`{"strategy":{"params":{"soc_steps":2.5}}}`, "strategy.params.soc_steps"},
		{`{"strategy":{"params":{"soc_steps":9223372036854775808}}}`, "strategy.params.soc_steps"},
		{`{"strategy":{"params":{"nope":0}}}`, "strategy.params.nope"},
		{`{"strategy":{"name":"schedule","params":{"soc_steps":2}}}`, "strategy.params.soc_steps"},
		{`{"strategy":{"name":"oracle","params":{"charge_start":"10:00"}}}`, "strategy.params.charge_start"},
	}
	for _, tc := range cases {
		for _, decode := range []func([]byte, any) error{json.Unmarshal, yaml.Unmarshal} {
			var input Input
			requireField(t, decode([]byte(tc.raw), &input), tc.field)
		}
	}
	for _, param := range []string{"charge_start", "charge_end", "discharge_start", "discharge_end", "charge_power_mw", "discharge_power_mw", "soc_steps", "power_steps"} {
		for _, decode := range []func([]byte, any) error{json.Unmarshal, yaml.Unmarshal} {
			var input Input
			requireField(t, decode([]byte(fmt.Sprintf(`{"strategy":{"params":{"%s":null}}}`, param)), &input), "strategy.params."+param)
		}
	}
	var input Input
	requireField(t, json.Unmarshal([]byte(`null`), &input), "")
	requireField(t, yaml.Unmarshal([]byte("battery:\n  min_soc: .nan\n"), &input), "battery.min_soc")
	if err := json.Unmarshal([]byte(`{"strategy":{"params":{"soc_steps":2}}}`), &input); err != nil || input.Strategy.Name != nil || *input.Strategy.Params.SOCSteps != 2 {
		t.Fatalf("partial variation: %+v %v", input, err)
	}
}

func TestMergeEveryBatteryFieldWithoutAliasing(t *testing.T) {
	base := fullInput()
	base.Battery.InitialSOC, base.Battery.DegradationCostPerMWh = ptr(.5), ptr(2.0)
	raw := `{"battery":{"name":"","energy_capacity_mwh":0,"power_capacity_mw":0,"charge_efficiency":0,"discharge_efficiency":0,"min_soc":0,"max_soc":0,"initial_soc":0,"degradation_cost_per_mwh":0}}`
	var patch Input
	if err := json.Unmarshal([]byte(raw), &patch); err != nil {
		t.Fatal(err)
	}
	out, err := MergeInput(base, patch)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out.Battery, patch.Battery) {
		t.Fatalf("zero overrides lost: %+v", out.Battery)
	}
	*out.Battery.MinSOC = .8
	*out.Strategy.Name = "oracle"
	if *patch.Battery.MinSOC != 0 || *base.Strategy.Name != "schedule" {
		t.Fatal("merged pointers alias caller data")
	}
}

func TestIntegerParametersUseNumericSemantics(t *testing.T) {
	for _, decode := range []func([]byte, any) error{json.Unmarshal, yaml.Unmarshal} {
		for _, numeric := range []string{"200", "200.0", "2e2"} {
			var input Input
			raw := fmt.Sprintf(`{"strategy":{"name":"oracle","params":{"soc_steps":%s}}}`, numeric)
			if err := decode([]byte(raw), &input); err != nil {
				t.Fatalf("%s: %v", numeric, err)
			}
			if input.Strategy.Params.SOCSteps == nil || *input.Strategy.Params.SOCSteps != 200 {
				t.Fatalf("numeric value changed: %+v", input.Strategy.Params)
			}
		}
		for _, numeric := range []string{"200.5", `"200"`, "9.223372036854776e18", "1e100"} {
			var input Input
			raw := fmt.Sprintf(`{"strategy":{"params":{"soc_steps":%s}}}`, numeric)
			requireField(t, decode([]byte(raw), &input), "strategy.params.soc_steps")
		}
	}
}

func TestStrategyParameterInheritanceAndChange(t *testing.T) {
	base := fullInput()
	base.Strategy.Params = StrategyParamsOverride{ChargeStart: ptr("09:00"), ChargePowerMW: ptr(5.0)}
	patch := Input{Strategy: StrategyOverride{Params: StrategyParamsOverride{DischargeStart: ptr("18:00"), ChargePowerMW: ptr(0.0)}}}
	merged, err := MergeInput(base, patch)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Resolve(merged, PresetCatalog{})
	if err != nil {
		t.Fatal(err)
	}
	got := out.Strategy.(ScheduleConfig)
	if got.ChargeStartMinute != 540 || got.ChargeEndMinute != 1080 || got.ChargePowerMW != 0 {
		t.Fatalf("inheritance: %+v", got)
	}
	patch.Strategy = StrategyOverride{Name: ptr("oracle"), Params: StrategyParamsOverride{PowerSteps: ptr(3)}}
	merged, err = MergeInput(base, patch)
	if err != nil {
		t.Fatal(err)
	}
	out, err = Resolve(merged, PresetCatalog{})
	if err != nil || out.Strategy != (OracleConfig{200, 3}) {
		t.Fatalf("changed strategy: %+v %v", out, err)
	}
	_, err = Resolve(Input{Battery: base.Battery}, PresetCatalog{})
	requireField(t, err, "strategy.name")
}

func TestScheduleWindowsAndPowers(t *testing.T) {
	for _, bad := range []string{"9:00", "24:00", "12:60", " 10:00", "10:00:00", "-1:00", "00:0x", "１０:00"} {
		in := fullInput()
		in.Strategy.Params.ChargeStart = ptr(bad)
		_, err := Resolve(in, PresetCatalog{})
		requireField(t, err, "strategy.params.charge_start")
	}
	for _, tc := range []struct {
		start, end, ds, de string
		idle               bool
		invalid            bool
	}{
		{"22:00", "02:00", "02:00", "06:00", false, false},
		{"22:00", "02:00", "01:00", "06:00", false, true},
		{"10:00", "18:00", "17:00", "23:59", false, true},
		{"10:00", "18:00", "17:00", "23:59", true, false},
		{"10:00", "10:00", "00:00", "23:59", false, false},
	} {
		in := fullInput()
		in.Strategy.Params = StrategyParamsOverride{ChargeStart: ptr(tc.start), ChargeEnd: ptr(tc.end), DischargeStart: ptr(tc.ds), DischargeEnd: ptr(tc.de)}
		if tc.idle {
			in.Strategy.Params.ChargePowerMW = ptr(0.0)
		}
		_, err := Resolve(in, PresetCatalog{})
		if (err != nil) != tc.invalid {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
	for _, bad := range []float64{-1, math.Inf(1), math.NaN()} {
		in := fullInput()
		in.Strategy.Params.DischargePowerMW = ptr(bad)
		_, err := Resolve(in, PresetCatalog{})
		requireField(t, err, "strategy.params.discharge_power_mw")
	}
	for _, tc := range []struct {
		soc, power int
		badField   string
	}{{1, 10, "soc_steps"}, {1001, 10, "soc_steps"}, {200, 0, "power_steps"}, {200, 51, "power_steps"}, {2, 1, ""}, {1000, 50, ""}} {
		in := fullInput()
		in.Strategy = StrategyOverride{Name: ptr("oracle"), Params: StrategyParamsOverride{SOCSteps: ptr(tc.soc), PowerSteps: ptr(tc.power)}}
		_, err := Resolve(in, PresetCatalog{})
		if tc.badField != "" {
			requireField(t, err, "strategy.params."+tc.badField)
		} else if err != nil {
			t.Fatal(err)
		}
	}
}

func TestMetadataDefaultsMatchResolver(t *testing.T) {
	for _, definition := range StrategyMetadata() {
		params := map[string]any{}
		for _, param := range definition.Parameters {
			if param.Default != nil {
				params[param.Name] = param.Default
			}
			if param.DefaultSource != "" {
				raw, _ := json.Marshal(param)
				if strings.Contains(string(raw), `"default":`) {
					t.Fatalf("derived default materialized: %s", raw)
				}
			}
		}
		raw, err := json.Marshal(map[string]any{"strategy": map[string]any{"name": definition.Name, "params": params}})
		if err != nil {
			t.Fatal(err)
		}
		var explicit Input
		if err := json.Unmarshal(raw, &explicit); err != nil {
			t.Fatal(err)
		}
		explicit.Battery = fullInput().Battery
		omitted := fullInput()
		omitted.Strategy.Name = ptr(definition.Name)
		a, err := Resolve(explicit, PresetCatalog{})
		if err != nil {
			t.Fatal(err)
		}
		b, err := Resolve(omitted, PresetCatalog{})
		if err != nil {
			t.Fatal(err)
		}
		if a != b {
			t.Fatalf("metadata differs: %+v %+v", a, b)
		}
	}
	metadata := StrategyMetadata()
	metadata[0].Parameters[0].Default = "00:00"
	metadata[0].Name = "bad"
	if StrategyMetadata()[0].Parameters[0].Default != "10:00" || StrategyMetadata()[0].Name != "schedule" {
		t.Fatal("metadata is mutable")
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}

func presetText(power float64, min float64) string {
	return fmt.Sprintf("battery:\n  name: preset\n  energy_capacity_mwh: 100\n  power_capacity_mw: %v\n  charge_efficiency: 1\n  discharge_efficiency: 1\n  min_soc: %v\n  max_soc: 0.9\n  degradation_cost_per_mwh: 5\n", power, min)
}

func TestCatalogAndChangedPresetPrecedence(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.yaml"), presetText(10, .1))
	writeFile(t, filepath.Join(dir, "b.yaml"), presetText(20, .2))
	catalog, err := LoadPresetCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	base := Input{BatteryFile: "a", Battery: BatteryOverride{DegradationCostPerMWh: ptr(0.0)}, Strategy: StrategyOverride{Name: ptr("schedule")}}
	merged, err := MergeInput(base, Input{BatteryFile: "b", Battery: BatteryOverride{MinSOC: ptr(0.0), InitialSOC: ptr(0.0)}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(merged, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if got.Battery.PowerCapacityMW != 20 || got.Battery.MinSOC != 0 || got.Battery.InitialSOC != 0 || got.Battery.DegradationCostPerMWh != 0 {
		t.Fatalf("preset precedence: %+v", got.Battery)
	}
	explicit := fullInput()
	explicit.Battery = BatteryOverride{Name: ptr("preset"), EnergyCapacityMWh: ptr(100.0), PowerCapacityMW: ptr(20.0), ChargeEfficiency: ptr(1.0), DischargeEfficiency: ptr(1.0), MinSOC: ptr(0.0), MaxSOC: ptr(.9), InitialSOC: ptr(0.0), DegradationCostPerMWh: ptr(0.0)}
	equivalent, err := Resolve(explicit, PresetCatalog{})
	if err != nil || got != equivalent {
		t.Fatalf("explicit equivalence: %+v %v", equivalent, err)
	}
	base.Battery.MinSOC = ptr(.3)
	got, err = Resolve(base, catalog)
	if err != nil || got.Battery.InitialSOC != .3 {
		t.Fatalf("preset absent initial SOC materialized: %+v %v", got.Battery, err)
	}
	listing := catalog.List()
	if len(listing) != 2 || listing[0].ID != "a" {
		t.Fatalf("listing: %+v", listing)
	}
	listing[0].Battery.PowerCapacityMW = 999
	writeFile(t, filepath.Join(dir, "a.yaml"), "corrupted after load")
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resolved, err := Resolve(base, catalog)
			if err != nil || resolved.Battery.PowerCapacityMW != 10 {
				t.Errorf("catalog mutation/read: %+v %v", resolved, err)
			}
			_ = catalog.List()
		}()
	}
	wg.Wait()
	for _, id := range []string{"../a", "a.yaml", "nested/a", ".", "/a", "a\\b", "unknown"} {
		base.BatteryFile = id
		_, err := Resolve(base, catalog)
		requireField(t, err, "battery_file")
	}
}

func TestCatalogFailsClosed(t *testing.T) {
	if _, err := LoadPresetCatalog(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing catalog: %v", err)
	}
	for _, raw := range []string{"battery_file: ../outside.yaml\n", presetText(10, .1) + "other: 1\n", "battery:\n  min_soc: 0\n", presetText(10, .1) + "---\nbattery: {}\n", "null\n"} {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "a.yaml"), raw)
		if _, err := LoadPresetCatalog(dir); err == nil {
			t.Fatalf("accepted malformed preset: %q", raw)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.yaml")
	writeFile(t, outside, presetText(10, .1))
	dir := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "a.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPresetCatalog(dir); err == nil {
		t.Fatal("followed preset symlink")
	}
}

func TestLoadFileRelativePathsAndNoDefaults(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "battery.yaml"), presetText(10, .1))
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, "battery_file: battery.yaml\nbattery:\n  min_soc: 0\n  degradation_cost_per_mwh: 0\nstrategy:\n  name: schedule\n")
	input, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if input.BatteryFile != "" || input.Battery.InitialSOC != nil || input.Strategy.Params.ChargeStart != nil || *input.Battery.MinSOC != 0 || *input.Battery.DegradationCostPerMWh != 0 {
		t.Fatalf("defaults/overrides: %+v", input)
	}
	resolved, err := Resolve(input, PresetCatalog{})
	if err != nil || resolved.Battery.InitialSOC != 0 {
		t.Fatalf("resolved: %+v %v", resolved, err)
	}
	for _, raw := range []string{"null\n", "battery_file: null\n", "battery_file: ''\n", "battery: null\n", "{}\n---\n{}\n"} {
		writeFile(t, path, raw)
		if _, err := LoadFile(path); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(nested, "config.yaml"), "battery_file: battery.yaml\nstrategy: {name: schedule}\n")
	t.Chdir(dir)
	if _, err := LoadFile(filepath.Join(nested, "config.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cwd fallback: %v", err)
	}
}

func TestExamplesFromUnrelatedWorkingDirectory(t *testing.T) {
	examples, err := filepath.Abs("../../examples")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	files, err := filepath.Glob(filepath.Join(examples, "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 example configurations, got %v", files)
	}
	for _, path := range files {
		input, err := LoadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if _, err := Resolve(input, PresetCatalog{}); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	catalog, err := LoadPresetCatalog(filepath.Join(examples, "batteries"))
	if err != nil || len(catalog.List()) != 5 {
		t.Fatalf("example catalog: %v", err)
	}
}
