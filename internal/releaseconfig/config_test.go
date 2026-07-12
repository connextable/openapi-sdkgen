package releaseconfig

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"go.yaml.in/yaml/v4"
)

type config struct {
	Version int `yaml:"version"`
	Builds  []struct {
		ID     string   `yaml:"id"`
		Main   string   `yaml:"main"`
		Binary string   `yaml:"binary"`
		GOOS   []string `yaml:"goos"`
		GOARCH []string `yaml:"goarch"`
	} `yaml:"builds"`
	Archives []struct {
		Formats   []string `yaml:"formats"`
		Overrides []struct {
			GOOS    string   `yaml:"goos"`
			Formats []string `yaml:"formats"`
		} `yaml:"format_overrides"`
	} `yaml:"archives"`
	Checksum struct {
		NameTemplate string `yaml:"name_template"`
	} `yaml:"checksum"`
}

func TestReleaseConfigurationMatchesSupportedBinaryContract(t *testing.T) {
	path := filepath.Join("..", "..", ".goreleaser.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value config
	if err := yaml.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	if value.Version != 2 || len(value.Builds) != 1 {
		t.Fatalf("release config version/builds = %#v", value)
	}
	build := value.Builds[0]
	if build.ID != "openapi-sdkgen" || build.Main != "./cmd/openapi-sdkgen" || build.Binary != "openapi-sdkgen" {
		t.Fatalf("build = %#v", build)
	}
	if !slices.Equal(build.GOOS, []string{"darwin", "linux", "windows"}) || !slices.Equal(build.GOARCH, []string{"amd64", "arm64"}) {
		t.Fatalf("platforms = %#v", build)
	}
	if len(value.Archives) != 1 || !slices.Equal(value.Archives[0].Formats, []string{"tar.gz"}) || len(value.Archives[0].Overrides) != 1 || value.Archives[0].Overrides[0].GOOS != "windows" || !slices.Equal(value.Archives[0].Overrides[0].Formats, []string{"zip"}) {
		t.Fatalf("archives = %#v", value.Archives)
	}
	if value.Checksum.NameTemplate != "checksums.txt" {
		t.Fatalf("checksum = %#v", value.Checksum)
	}
}
