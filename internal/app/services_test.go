package app

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/infra/geoip"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// optionalDependencies are the dependency fields the application leaves nil
// on purpose, each with the reason.
var optionalDependencies = map[string]string{
	// The task worker's aggregator only flushes buckets; these apply to
	// accepting a report, which the node API does with its own aggregator.
	"tasks.Traffic.Aggregator.TrafficReportThreshold": "report-time only",
	"tasks.Traffic.Aggregator.Multiplier":             "report-time only",
	"tasks.Traffic.Aggregator.ServedSubscriptions":    "report-time only",
}

// The composition root wires every port: a dependency left nil would only
// surface as a panic when the first request or task reached it. The graph is
// assembled on in-memory infrastructure, so nothing connects anywhere.
func TestAssembledDependenciesAreComplete(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	mini := miniredis.RunT(t)
	rds := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rds.Close() })
	var c config.Config
	c.Redis.Host = mini.Addr()
	queue, inspector := NewAsynqClient(c), NewAsynqInspector(c)
	t.Cleanup(func() {
		_ = queue.Close()
		_ = inspector.Close()
	})

	srv := assemble(c, db, rds, &geoip.IPLocation{}, queue, inspector)
	service := srv.serviceDependencies()
	for name, deps := range map[string]any{
		"application": *srv,
		"service":     service,
		"bootstrap":   *service.Bootstrap,
		"http":        service.HTTP(),
		"tasks":       srv.taskDependencies(),
	} {
		for _, field := range unwired(reflect.ValueOf(deps), name) {
			if _, optional := optionalDependencies[field]; !optional {
				t.Errorf("%s is not wired", field)
			}
		}
	}
}

// unwired lists the nil dependency fields of v, a struct, named path. It
// descends into nested structs, not into what pointers and interfaces hold.
func unwired(v reflect.Value, path string) []string {
	var nils []string
	for i := 0; i < v.NumField(); i++ {
		field, name := v.Field(i), path+"."+v.Type().Field(i).Name
		switch field.Kind() {
		case reflect.Struct:
			if strings.HasPrefix(field.Type().PkgPath(), "github.com/perfect-panel/server/") && !isConfigValue(field.Type()) {
				nils = append(nils, unwired(field, name)...)
			}
		case reflect.Pointer, reflect.Interface, reflect.Func, reflect.Map, reflect.Chan:
			if field.IsNil() {
				nils = append(nils, strings.TrimPrefix(name, "http."))
			}
		}
	}
	return nils
}

// isConfigValue reports whether t is configuration data rather than a set of
// dependencies: its zero fields are settings, not missing wiring.
func isConfigValue(t reflect.Type) bool {
	return t.PkgPath() == "github.com/perfect-panel/server/internal/config" || strings.HasSuffix(fmt.Sprint(t), "Snapshot")
}
