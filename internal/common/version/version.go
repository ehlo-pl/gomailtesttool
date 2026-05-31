package version

// Version is the current version of the tool suite.
// This is the single source of truth for versioning across all tools.
// All tools (msgraphtool, smtptool) share the same version number.
//
// To update the version:
// 1. Change the Version constant below
// 2. Create a changelog entry in ChangeLog/{version}.md
// 3. Commit with message: "Bump version to {version}"
const Version = "3.3.8"

// Get returns the current version string.
// This is a convenience function for accessing the Version constant.
func Get() string {
	return Version
}
