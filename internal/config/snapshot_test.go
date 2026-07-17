package config

import (
	"reflect"
	"testing"
)

func TestSnapshotDeepCopiesMutableConfiguration(t *testing.T) {
	source := Config{
		Logging: LoggingConfig{AllowedExtensions: []string{".log", ".json"}},
		CORS: CORSConfig{
			AllowOrigins:  []string{"https://admin.example.com"},
			AllowMethods:  []string{"GET", "POST"},
			AllowHeaders:  []string{"Authorization"},
			ExposeHeaders: []string{"X-Request-Id"},
		},
	}
	want := source
	want.Logging.AllowedExtensions = append([]string(nil), source.Logging.AllowedExtensions...)
	want.CORS.AllowOrigins = append([]string(nil), source.CORS.AllowOrigins...)
	want.CORS.AllowMethods = append([]string(nil), source.CORS.AllowMethods...)
	want.CORS.AllowHeaders = append([]string(nil), source.CORS.AllowHeaders...)
	want.CORS.ExposeHeaders = append([]string(nil), source.CORS.ExposeHeaders...)

	snapshot := Snapshot(source)
	source.Logging.AllowedExtensions[0] = ".mutated"
	source.CORS.AllowOrigins[0] = "https://mutated.example.com"
	source.CORS.AllowMethods[0] = "DELETE"
	source.CORS.AllowHeaders[0] = "Cookie"
	source.CORS.ExposeHeaders[0] = "Server"
	if !reflect.DeepEqual(snapshot, want) {
		t.Fatalf("snapshot changed with source mutation:\n got=%#v\nwant=%#v", snapshot, want)
	}

	snapshot.CORS.AllowOrigins[0] = "https://snapshot.example.com"
	if source.CORS.AllowOrigins[0] == snapshot.CORS.AllowOrigins[0] {
		t.Fatal("snapshot mutation changed source configuration")
	}
}
