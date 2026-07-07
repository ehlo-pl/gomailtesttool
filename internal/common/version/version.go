package version

// Version is the current version of the tool suite.
// This is the single source of truth for versioning across all tools.
// Every protocol subcommand of the gomailtest binary shares this version.
//
// To update the version:
// 1. Change the Version constant below
// 2. Create a changelog entry in ChangeLog/{version}.md
// 3. Commit with message: "Bump version to {version}"
const Version = "3.5.0"

// Get returns the current version string.
// This is a convenience function for accessing the Version constant.
func Get() string {
	return Version
}
