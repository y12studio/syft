package source

import (
	"io"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/scylladb/go-set/strset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anchore/stereoscope/pkg/file"
	"github.com/anchore/stereoscope/pkg/imagetest"
)

func TestImageSquashResolver_FilesByPath(t *testing.T) {
	cases := []struct {
		name                 string
		linkPath             string
		resolveLayer         uint
		resolvePath          string
		forcePositiveHasPath bool
	}{
		{
			name:         "link with previous data",
			linkPath:     "/link-1",
			resolveLayer: 1,
			resolvePath:  "/file-1.txt",
		},
		{
			name:         "link with in layer data",
			linkPath:     "/link-within",
			resolveLayer: 5,
			resolvePath:  "/file-3.txt",
		},
		{
			name:         "link with overridden data",
			linkPath:     "/link-2",
			resolveLayer: 7,
			resolvePath:  "/file-2.txt",
		},
		{
			name:         "indirect link (with overridden data)",
			linkPath:     "/link-indirect",
			resolveLayer: 7,
			resolvePath:  "/file-2.txt",
		},
		{
			name:         "dead link",
			linkPath:     "/link-dead",
			resolveLayer: 8,
			resolvePath:  "",
			// the path should exist, even if the link is dead
			forcePositiveHasPath: true,
		},
		{
			name:        "ignore directories",
			linkPath:    "/bin",
			resolvePath: "",
			// the path should exist, even if we ignore it
			forcePositiveHasPath: true,
		},
		{
			name:         "parent is a link (with overridden data)",
			linkPath:     "/parent-link/file-4.txt",
			resolveLayer: 11,
			resolvePath:  "/parent/file-4.txt",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			img := imagetest.GetFixtureImage(t, "docker-archive", "image-symlinks")

			resolver, err := newImageSquashResolver(img)
			if err != nil {
				t.Fatalf("could not create resolver: %+v", err)
			}

			hasPath := resolver.HasPath(c.linkPath)
			if !c.forcePositiveHasPath {
				if c.resolvePath != "" && !hasPath {
					t.Errorf("expected HasPath() to indicate existance, but did not")
				} else if c.resolvePath == "" && hasPath {
					t.Errorf("expeced HasPath() to NOT indicate existance, but does")
				}
			} else if !hasPath {
				t.Errorf("expected HasPath() to indicate existance, but did not (force path)")
			}

			refs, err := resolver.FilesByPath(c.linkPath)
			if err != nil {
				t.Fatalf("could not use resolver: %+v", err)
			}

			expectedRefs := 1
			if c.resolvePath == "" {
				expectedRefs = 0
			}

			if len(refs) != expectedRefs {
				t.Fatalf("unexpected number of resolutions: %d", len(refs))
			}

			if expectedRefs == 0 {
				// nothing else to assert
				return
			}

			actual := refs[0]

			if string(actual.ref.RealPath) != c.resolvePath {
				t.Errorf("bad resolve path: '%s'!='%s'", string(actual.ref.RealPath), c.resolvePath)
			}

			if c.resolvePath != "" && string(actual.ref.RealPath) != actual.RealPath {
				t.Errorf("we should always prefer real paths over ones with links")
			}

			layer := img.FileCatalog.Layer(actual.ref)

			if layer.Metadata.Index != c.resolveLayer {
				t.Errorf("bad resolve layer: '%d'!='%d'", layer.Metadata.Index, c.resolveLayer)
			}
		})
	}
}

func TestImageSquashResolver_FilesByGlob(t *testing.T) {
	cases := []struct {
		name         string
		glob         string
		resolveLayer uint
		resolvePath  string
	}{
		{
			name:         "link with previous data",
			glob:         "**/link-1",
			resolveLayer: 1,
			resolvePath:  "/file-1.txt",
		},
		{
			name:         "link with in layer data",
			glob:         "**/link-within",
			resolveLayer: 5,
			resolvePath:  "/file-3.txt",
		},
		{
			name:         "link with overridden data",
			glob:         "**/link-2",
			resolveLayer: 7,
			resolvePath:  "/file-2.txt",
		},
		{
			name:         "indirect link (with overridden data)",
			glob:         "**/link-indirect",
			resolveLayer: 7,
			resolvePath:  "/file-2.txt",
		},
		{
			name: "dead link",
			glob: "**/link-dead",
			// dead links are dead! they shouldn't match on globs
			resolvePath: "",
		},
		{
			name:        "ignore directories",
			glob:        "**/bin",
			resolvePath: "",
		},
		{
			name:         "parent without link",
			glob:         "**/parent/*.txt",
			resolveLayer: 11,
			resolvePath:  "/parent/file-4.txt",
		},
		{
			name:         "parent is a link (override)",
			glob:         "**/parent-link/file-4.txt",
			resolveLayer: 11,
			resolvePath:  "/parent/file-4.txt",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			img := imagetest.GetFixtureImage(t, "docker-archive", "image-symlinks")

			resolver, err := newImageSquashResolver(img)
			if err != nil {
				t.Fatalf("could not create resolver: %+v", err)
			}

			refs, err := resolver.FilesByGlob(c.glob)
			if err != nil {
				t.Fatalf("could not use resolver: %+v", err)
			}

			expectedRefs := 1
			if c.resolvePath == "" {
				expectedRefs = 0
			}

			if len(refs) != expectedRefs {
				t.Fatalf("unexpected number of resolutions: %d", len(refs))
			}

			if expectedRefs == 0 {
				// nothing else to assert
				return
			}

			actual := refs[0]

			if string(actual.ref.RealPath) != c.resolvePath {
				t.Errorf("bad resolve path: '%s'!='%s'", string(actual.ref.RealPath), c.resolvePath)
			}

			if c.resolvePath != "" && string(actual.ref.RealPath) != actual.RealPath {
				t.Errorf("we should always prefer real paths over ones with links")
			}

			layer := img.FileCatalog.Layer(actual.ref)

			if layer.Metadata.Index != c.resolveLayer {
				t.Errorf("bad resolve layer: '%d'!='%d'", layer.Metadata.Index, c.resolveLayer)
			}
		})
	}
}

func Test_imageSquashResolver_FilesByMIMEType(t *testing.T) {

	tests := []struct {
		fixtureName   string
		mimeType      string
		expectedPaths *strset.Set
	}{
		{
			fixtureName:   "image-simple",
			mimeType:      "text/plain",
			expectedPaths: strset.New("/somefile-1.txt", "/somefile-2.txt", "/really/nested/file-3.txt"),
		},
	}

	for _, test := range tests {
		t.Run(test.fixtureName, func(t *testing.T) {
			img := imagetest.GetFixtureImage(t, "docker-archive", test.fixtureName)

			resolver, err := newImageSquashResolver(img)
			assert.NoError(t, err)

			locations, err := resolver.FilesByMIMEType(test.mimeType)
			assert.NoError(t, err)

			assert.Len(t, locations, test.expectedPaths.Size())
			for _, l := range locations {
				assert.True(t, test.expectedPaths.Has(l.RealPath), "does not have path %q", l.RealPath)
			}
		})
	}
}

func Test_imageSquashResolver_hasFilesystemIDInLocation(t *testing.T) {
	img := imagetest.GetFixtureImage(t, "docker-archive", "image-duplicate-path")

	resolver, err := newImageSquashResolver(img)
	assert.NoError(t, err)

	locations, err := resolver.FilesByMIMEType("text/plain")
	assert.NoError(t, err)
	assert.NotEmpty(t, locations)
	for _, location := range locations {
		assert.NotEmpty(t, location.FileSystemID)
	}

	locations, err = resolver.FilesByGlob("*.txt")
	assert.NoError(t, err)
	assert.NotEmpty(t, locations)
	for _, location := range locations {
		assert.NotEmpty(t, location.FileSystemID)
	}

	locations, err = resolver.FilesByPath("/somefile-1.txt")
	assert.NoError(t, err)
	assert.NotEmpty(t, locations)
	for _, location := range locations {
		assert.NotEmpty(t, location.FileSystemID)
	}

}

func TestSquashImageResolver_FilesContents(t *testing.T) {

	tests := []struct {
		name     string
		path     string
		contents []string
	}{
		{
			name: "one degree",
			path: "link-2",
			contents: []string{
				"NEW file override!", // always from the squashed perspective
			},
		},
		{
			name: "two degrees",
			path: "link-indirect",
			contents: []string{
				"NEW file override!", // always from the squashed perspective
			},
		},
		{
			name:     "dead link",
			path:     "link-dead",
			contents: []string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			img := imagetest.GetFixtureImage(t, "docker-archive", "image-symlinks")

			resolver, err := newImageSquashResolver(img)
			assert.NoError(t, err)

			refs, err := resolver.FilesByPath(test.path)
			require.NoError(t, err)
			assert.Len(t, refs, len(test.contents))

			for idx, loc := range refs {

				reader, err := resolver.FileContentsByLocation(loc)
				require.NoError(t, err)

				actual, err := io.ReadAll(reader)
				require.NoError(t, err)

				assert.Equal(t, test.contents[idx], string(actual))
			}
		})
	}
}

func TestSquashImageResolver_FilesContents_errorOnDirRequest(t *testing.T) {

	img := imagetest.GetFixtureImage(t, "docker-archive", "image-symlinks")

	resolver, err := newImageSquashResolver(img)
	assert.NoError(t, err)

	var dirLoc *Location
	for loc := range resolver.AllLocations() {
		entry, err := resolver.img.FileCatalog.Get(loc.ref)
		require.NoError(t, err)
		if entry.Metadata.IsDir {
			dirLoc = &loc
			break
		}
	}

	require.NotNil(t, dirLoc)

	reader, err := resolver.FileContentsByLocation(*dirLoc)
	require.Error(t, err)
	require.Nil(t, reader)
}

func Test_imageSquashResolver_resolvesLinks(t *testing.T) {
	tests := []struct {
		name     string
		runner   func(FileResolver) []Location
		expected []Location
	}{
		{
			name: "by mimetype",
			runner: func(resolver FileResolver) []Location {
				// links should not show up when searching mimetype
				actualLocations, err := resolver.FilesByMIMEType("text/plain")
				assert.NoError(t, err)
				return actualLocations
			},
			expected: []Location{
				{
					Coordinates: Coordinates{
						RealPath: "/etc/group",
					},
					VirtualPath: "/etc/group",
				},
				{
					Coordinates: Coordinates{
						RealPath: "/etc/passwd",
					},
					VirtualPath: "/etc/passwd",
				},
				{
					Coordinates: Coordinates{
						RealPath: "/etc/shadow",
					},
					VirtualPath: "/etc/shadow",
				},
				{
					Coordinates: Coordinates{
						RealPath: "/file-1.txt",
					},
					VirtualPath: "/file-1.txt",
				},
				{
					Coordinates: Coordinates{
						RealPath: "/file-3.txt",
					},
					VirtualPath: "/file-3.txt",
				},
				{
					Coordinates: Coordinates{
						RealPath: "/file-2.txt",
					},
					VirtualPath: "/file-2.txt",
				},
				{
					Coordinates: Coordinates{
						RealPath: "/parent/file-4.txt",
					},
					VirtualPath: "/parent/file-4.txt",
				},
			},
		},
		{
			name: "by glob to links",
			runner: func(resolver FileResolver) []Location {
				// links are searched, but resolve to the real files
				actualLocations, err := resolver.FilesByGlob("*ink-*")
				assert.NoError(t, err)
				return actualLocations
			},
			expected: []Location{
				{
					Coordinates: Coordinates{
						RealPath: "/file-1.txt",
					},
					VirtualPath: "/link-1",
				},
				{
					Coordinates: Coordinates{
						RealPath: "/file-2.txt",
					},
					VirtualPath: "/link-2",
				},
				// though this is a link, and it matches to the file, the resolver de-duplicates files
				// by the real path, so it is not included in the results
				//{
				//	Coordinates: Coordinates{
				//		RealPath: "/file-2.txt",
				//	},
				//	VirtualPath: "/link-indirect",
				//},
				{
					Coordinates: Coordinates{
						RealPath: "/file-3.txt",
					},
					VirtualPath: "/link-within",
				},
			},
		},
		{
			name: "by basename",
			runner: func(resolver FileResolver) []Location {
				// links are searched, but resolve to the real files
				actualLocations, err := resolver.FilesByGlob("**/file-2.txt")
				assert.NoError(t, err)
				return actualLocations
			},
			expected: []Location{
				// this has two copies in the base image, which overwrites the same location
				{
					Coordinates: Coordinates{
						RealPath: "/file-2.txt",
					},
					VirtualPath: "/file-2.txt",
				},
			},
		},
		{
			name: "by basename glob",
			runner: func(resolver FileResolver) []Location {
				// links are searched, but resolve to the real files
				actualLocations, err := resolver.FilesByGlob("**/file-?.txt")
				assert.NoError(t, err)
				return actualLocations
			},
			expected: []Location{
				{
					Coordinates: Coordinates{
						RealPath: "/file-1.txt",
					},
					VirtualPath: "/file-1.txt",
				},
				{
					Coordinates: Coordinates{
						RealPath: "/file-2.txt",
					},
					VirtualPath: "/file-2.txt",
				},
				{
					Coordinates: Coordinates{
						RealPath: "/file-3.txt",
					},
					VirtualPath: "/file-3.txt",
				},
				{
					Coordinates: Coordinates{
						RealPath: "/parent/file-4.txt",
					},
					VirtualPath: "/parent/file-4.txt",
				},
			},
		},
		{
			name: "by basename glob to links",
			runner: func(resolver FileResolver) []Location {
				actualLocations, err := resolver.FilesByGlob("**/link-*")
				assert.NoError(t, err)
				return actualLocations
			},
			expected: []Location{
				{
					Coordinates: Coordinates{
						RealPath: "/file-1.txt",
					},
					VirtualPath: "/link-1",
					ref:         file.Reference{RealPath: "/file-1.txt"},
				},
				{
					Coordinates: Coordinates{
						RealPath: "/file-2.txt",
					},
					VirtualPath: "/link-2",
					ref:         file.Reference{RealPath: "/file-2.txt"},
				},
				// we already have this real file path via another link, so only one is returned
				//{
				//	Coordinates: Coordinates{
				//		RealPath: "/file-2.txt",
				//	},
				//	VirtualPath: "/link-indirect",
				//	ref:         file.Reference{RealPath: "/file-2.txt"},
				//},
				{
					Coordinates: Coordinates{
						RealPath: "/file-3.txt",
					},
					VirtualPath: "/link-within",
					ref:         file.Reference{RealPath: "/file-3.txt"},
				},
			},
		},
		{
			name: "by extension",
			runner: func(resolver FileResolver) []Location {
				// links are searched, but resolve to the real files
				actualLocations, err := resolver.FilesByGlob("**/*.txt")
				assert.NoError(t, err)
				return actualLocations
			},
			expected: []Location{
				{
					Coordinates: Coordinates{
						RealPath: "/file-1.txt",
					},
					VirtualPath: "/file-1.txt",
				},
				{
					Coordinates: Coordinates{
						RealPath: "/file-2.txt",
					},
					VirtualPath: "/file-2.txt",
				},
				{
					Coordinates: Coordinates{
						RealPath: "/file-3.txt",
					},
					VirtualPath: "/file-3.txt",
				},
				{
					Coordinates: Coordinates{
						RealPath: "/parent/file-4.txt",
					},
					VirtualPath: "/parent/file-4.txt",
				},
			},
		},
		{
			name: "by path to degree 1 link",
			runner: func(resolver FileResolver) []Location {
				// links resolve to the final file
				actualLocations, err := resolver.FilesByPath("/link-2")
				assert.NoError(t, err)
				return actualLocations
			},
			expected: []Location{
				// we have multiple copies across layers
				{
					Coordinates: Coordinates{
						RealPath: "/file-2.txt",
					},
					VirtualPath: "/link-2",
				},
			},
		},
		{
			name: "by path to degree 2 link",
			runner: func(resolver FileResolver) []Location {
				// multiple links resolves to the final file
				actualLocations, err := resolver.FilesByPath("/link-indirect")
				assert.NoError(t, err)
				return actualLocations
			},
			expected: []Location{
				// we have multiple copies across layers
				{
					Coordinates: Coordinates{
						RealPath: "/file-2.txt",
					},
					VirtualPath: "/link-indirect",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			img := imagetest.GetFixtureImage(t, "docker-archive", "image-symlinks")

			resolver, err := newImageSquashResolver(img)
			assert.NoError(t, err)

			actual := test.runner(resolver)

			compareLocations(t, test.expected, actual)
		})
	}

}

func compareLocations(t *testing.T, expected, actual []Location) {
	t.Helper()
	ignoreUnexported := cmpopts.IgnoreFields(Location{}, "ref")
	ignoreFS := cmpopts.IgnoreFields(Coordinates{}, "FileSystemID")

	sort.Sort(Locations(expected))
	sort.Sort(Locations(actual))

	if d := cmp.Diff(expected, actual,
		ignoreUnexported,
		ignoreFS,
	); d != "" {

		t.Errorf("unexpected locations (-want +got):\n%s", d)
	}

}

func TestSquashResolver_AllLocations(t *testing.T) {
	img := imagetest.GetFixtureImage(t, "docker-archive", "image-files-deleted")

	resolver, err := newImageSquashResolver(img)
	assert.NoError(t, err)

	paths := strset.New()
	for loc := range resolver.AllLocations() {
		paths.Add(loc.RealPath)
	}
	expected := []string{
		"/Dockerfile",
		"/file-3.txt",
		"/target",
		"/target/file-2.txt",
	}

	// depending on how the image is built (either from linux or mac), sys and proc might accidentally be added to the image.
	// this isn't important for the test, so we remove them.
	paths.Remove("/proc", "/sys", "/dev", "/etc")

	pathsList := paths.List()
	sort.Strings(pathsList)

	assert.ElementsMatchf(t, expected, pathsList, "expected all paths to be indexed, but found different paths: \n%s", cmp.Diff(expected, paths.List()))
}
