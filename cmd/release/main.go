// Command release bumps the semver tag, updates VERSION, and pushes.
//
// Usage:
//
//	go run ./cmd/release                 # patch bump (default)
//	go run ./cmd/release -bump=minor
//	go run ./cmd/release -bump=major
//	go run ./cmd/release -version=v0.2.0 # explicit
//	go run ./cmd/release -dry-run
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var semverRE = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)

func main() {
	bump := flag.String("bump", "patch", "Version bump: patch, minor, or major (ignored if -version is set)")
	version := flag.String("version", "", "Explicit version (e.g. v0.2.0)")
	dryRun := flag.Bool("dry-run", false, "Print actions without tagging or pushing")
	skipPush := flag.Bool("skip-push", false, "Create local tag/commit but do not push")
	allowDirty := flag.Bool("allow-dirty", false, "Allow uncommitted changes (not recommended)")
	flag.Parse()

	root, err := moduleRoot()
	if err != nil {
		fatalf("%v", err)
	}
	if err := os.Chdir(root); err != nil {
		fatalf("chdir: %v", err)
	}

	_ = gitRun("fetch", "--tags", "--quiet")

	current := latestTag()
	next, err := nextVersion(current, *bump, *version)
	if err != nil {
		fatalf("%v", err)
	}

	fmt.Printf("Current tag: %s\n", displayTag(current))
	fmt.Printf("Next tag:    %s\n", next)

	if *dryRun {
		fmt.Println("Dry run — no commit, tag, or push.")
		return
	}

	if !*allowDirty {
		if out := gitOutput("status", "--porcelain"); strings.TrimSpace(out) != "" {
			fatalf("working tree is dirty; commit or stash first (or pass -allow-dirty):\n%s", out)
		}
	}

	versionPath := filepath.Join(root, "VERSION")
	// VERSION is a public semver marker, not a secret — 0644 is intentional.
	if err := os.WriteFile(versionPath, []byte(next+"\n"), 0o644); err != nil { //nolint:gosec // G306: non-secret project file
		fatalf("write VERSION: %v", err)
	}

	if err := gitRun("add", "VERSION"); err != nil {
		fatalf("git add: %v", err)
	}
	// Only commit if VERSION changed.
	if strings.TrimSpace(gitOutput("status", "--porcelain", "VERSION")) != "" {
		msg := "chore: release " + next
		if err := gitRun("commit", "-m", msg); err != nil {
			fatalf("git commit: %v", err)
		}
		fmt.Println("Committed VERSION update.")
	} else {
		fmt.Println("VERSION already at", next)
	}

	if err := gitRun("tag", "-a", next, "-m", "Release "+next); err != nil {
		fatalf("git tag: %v", err)
	}
	fmt.Println("Created tag", next)

	if *skipPush {
		fmt.Println("Skipped push (-skip-push).")
		return
	}

	if err := gitRun("push", "origin", "HEAD"); err != nil {
		fatalf("git push HEAD: %v", err)
	}
	if err := gitRun("push", "origin", next); err != nil {
		fatalf("git push tag: %v", err)
	}
	fmt.Printf("Pushed HEAD and %s — GitHub Release workflow should start.\n", next)
}

func displayTag(tag string) string {
	if tag == "" {
		return "(none)"
	}
	return tag
}

func latestTag() string {
	out := strings.TrimSpace(gitOutput("tag", "-l", "v*", "--sort=-v:refname"))
	if out == "" {
		return ""
	}
	return strings.Split(out, "\n")[0]
}

func nextVersion(current, bump, explicit string) (string, error) {
	if explicit != "" {
		m := semverRE.FindStringSubmatch(strings.TrimSpace(explicit))
		if m == nil {
			return "", fmt.Errorf("invalid -version %q (want vMAJOR.MINOR.PATCH)", explicit)
		}
		return "v" + m[1] + "." + m[2] + "." + m[3], nil
	}

	major, minor, patch := 0, 0, 0
	if current != "" {
		m := semverRE.FindStringSubmatch(current)
		if m == nil {
			return "", fmt.Errorf("latest tag %q is not semver; pass -version=vX.Y.Z", current)
		}
		major, _ = strconv.Atoi(m[1])
		minor, _ = strconv.Atoi(m[2])
		patch, _ = strconv.Atoi(m[3])
	}

	switch strings.ToLower(bump) {
	case "patch", "":
		patch++
	case "minor":
		minor++
		patch = 0
	case "major":
		major++
		minor = 0
		patch = 0
	default:
		return "", fmt.Errorf("invalid -bump %q (want patch, minor, or major)", bump)
	}
	return fmt.Sprintf("v%d.%d.%d", major, minor, patch), nil
}

func moduleRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found from %s", wd)
		}
		dir = parent
	}
}

func gitOutput(args ...string) string {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return string(out)
}

func gitRun(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}
