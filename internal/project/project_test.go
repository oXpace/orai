// Covers project.Locate, Project.id, and SetupTarget, including the case where this
// repository's own project root must be preferred over a config found in an ancestor
// directory (a project-package concern, tested here rather than in internal/setup).
package project_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/oXpace/orai/internal/project"
	"github.com/oXpace/orai/internal/state"
)

const minimalConfig = "schema = 1\nsession = \"orai\"\n"

// resolvePath mirrors the symlink+absolute resolution project.Locate/Load apply to a
// root. Every test resolves t.TempDir() through it up front, because on macOS
// t.TempDir() can return a path that is itself a symlink target
// (/var/folders/... -> /private/var/folders/...), which would otherwise make a raw,
// unresolved path string compare unequal to project.Project.Root.
func resolvePath(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)
	cmd := exec.Command("git", full...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func writeProject(t *testing.T, root, text string, gitInit bool) *project.Project {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if gitInit {
		git(t, root, "init", "-q", "-b", "trunk")
	}
	if err := os.WriteFile(filepath.Join(root, project.ConfigName), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := project.Load(root)
	if err != nil {
		t.Fatalf("project.Load(%s): %v", root, err)
	}
	return p
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
	if err != nil {
		t.Fatalf("copyTree(%s, %s): %v", src, dst, err)
	}
}

// --- Locate ----------------------------------------------------------------------

func TestLocateFromNestedSubdirectoryFindsRoot(t *testing.T) {
	tmp := resolvePath(t, t.TempDir())
	root := filepath.Join(tmp, "proj")
	writeProject(t, root, minimalConfig, false)
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := project.Locate(nested)
	if err != nil {
		t.Fatal(err)
	}
	if got != resolvePath(t, root) {
		t.Fatalf("Locate(nested) = %q, want %q", got, resolvePath(t, root))
	}
}

func TestLocateRaisesWhenNoConfigAnywhere(t *testing.T) {
	tmp := resolvePath(t, t.TempDir())
	empty := filepath.Join(tmp, "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := project.Locate(empty)
	if err == nil || !project.IsNotFound(err) {
		t.Fatalf("Locate(empty) error = %v, want NotFoundError", err)
	}
}

func TestLocateNonGitProjectWorks(t *testing.T) {
	tmp := resolvePath(t, t.TempDir())
	root := filepath.Join(tmp, "proj")
	writeProject(t, root, minimalConfig, false)
	got, err := project.Locate(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != resolvePath(t, root) {
		t.Fatalf("Locate(root) = %q, want %q", got, resolvePath(t, root))
	}
}

func TestLocateFromWorktreeReturnsMainCheckoutWhenMainHasConfig(t *testing.T) {
	tmp := resolvePath(t, t.TempDir())
	mainRoot := filepath.Join(tmp, "repo")
	writeProject(t, mainRoot, minimalConfig, true)
	git(t, mainRoot, "add", "orai.toml")
	git(t, mainRoot, "commit", "-q", "-m", "add config")

	worktree := filepath.Join(tmp, "repo-role")
	git(t, mainRoot, "worktree", "add", "-q", "-b", "feature", worktree, "trunk")
	if _, err := os.Stat(filepath.Join(worktree, "orai.toml")); err != nil {
		t.Fatalf("worktree config missing: %v", err)
	}

	got, err := project.Locate(worktree)
	if err != nil {
		t.Fatal(err)
	}
	if got != resolvePath(t, mainRoot) {
		t.Fatalf("Locate(worktree) = %q, want %q", got, resolvePath(t, mainRoot))
	}
}

func TestLocateFromWorktreeIsRootWhenMainLacksConfig(t *testing.T) {
	tmp := resolvePath(t, t.TempDir())
	mainRoot := filepath.Join(tmp, "repo2")
	if err := os.MkdirAll(mainRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, mainRoot, "init", "-q", "-b", "trunk")
	if err := os.WriteFile(filepath.Join(mainRoot, "README.md"), []byte("placeholder\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, mainRoot, "add", "README.md")
	git(t, mainRoot, "commit", "-q", "-m", "init")

	worktree := filepath.Join(tmp, "repo2-role")
	git(t, mainRoot, "worktree", "add", "-q", "-b", "feature", worktree, "trunk")
	if err := os.WriteFile(filepath.Join(worktree, "orai.toml"), []byte(minimalConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, worktree, "add", "orai.toml")
	git(t, worktree, "commit", "-q", "-m", "add config only on feature")

	if _, err := os.Stat(filepath.Join(mainRoot, "orai.toml")); err == nil {
		t.Fatalf("main checkout unexpectedly has orai.toml")
	}
	got, err := project.Locate(worktree)
	if err != nil {
		t.Fatal(err)
	}
	if got != resolvePath(t, worktree) {
		t.Fatalf("Locate(worktree) = %q, want %q", got, resolvePath(t, worktree))
	}
}

// --- Project.ID --------------------------------------------------------------------

func TestProjectIDIsStableForSamePath(t *testing.T) {
	tmp := resolvePath(t, t.TempDir())
	root := filepath.Join(tmp, "proj")
	proj := writeProject(t, root, minimalConfig, false)
	again, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if again.ID() != proj.ID() {
		t.Fatalf("ID() changed across loads: %q vs %q", again.ID(), proj.ID())
	}
}

func TestProjectIDDiffersForSameFolderNameInDifferentParents(t *testing.T) {
	tmp := resolvePath(t, t.TempDir())
	first := writeProject(t, filepath.Join(tmp, "a", "myproj"), minimalConfig, false)
	second := writeProject(t, filepath.Join(tmp, "b", "myproj"), minimalConfig, false)
	if first.ID() == second.ID() {
		t.Fatalf("IDs should differ: %q", first.ID())
	}
}

func TestProjectIDFromWorktreeMatchesMain(t *testing.T) {
	tmp := resolvePath(t, t.TempDir())
	mainRoot := filepath.Join(tmp, "repo")
	writeProject(t, mainRoot, minimalConfig, true)
	git(t, mainRoot, "add", "orai.toml")
	git(t, mainRoot, "commit", "-q", "-m", "add config")

	worktree := filepath.Join(tmp, "repo-role")
	git(t, mainRoot, "worktree", "add", "-q", "-b", "feature", worktree, "trunk")

	mainProject, err := project.Load(mainRoot)
	if err != nil {
		t.Fatal(err)
	}
	worktreeProject, err := project.Load(worktree)
	if err != nil {
		t.Fatal(err)
	}
	if worktreeProject.Root != resolvePath(t, mainRoot) {
		t.Fatalf("worktreeProject.Root = %q, want %q", worktreeProject.Root, resolvePath(t, mainRoot))
	}
	if worktreeProject.ID() != mainProject.ID() {
		t.Fatalf("IDs differ: %q vs %q", worktreeProject.ID(), mainProject.ID())
	}
}

func TestProjectIDAndRecordedRootAfterCopy(t *testing.T) {
	tmp := resolvePath(t, t.TempDir())
	originalRoot := filepath.Join(tmp, "original")
	writeProject(t, originalRoot, minimalConfig, false)
	if err := state.WriteJSON(filepath.Join(originalRoot, ".orai", "project.json"), map[string]any{"root": originalRoot}); err != nil {
		t.Fatal(err)
	}

	original, err := project.Load(originalRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got := original.RecordedRoot(); got != "" {
		t.Fatalf("RecordedRoot() = %q, want empty", got)
	}

	copyRoot := filepath.Join(tmp, "copy")
	copyTree(t, originalRoot, copyRoot)

	copied, err := project.Load(copyRoot)
	if err != nil {
		t.Fatal(err)
	}
	if copied.ID() == original.ID() {
		t.Fatalf("copied ID should differ from original")
	}
	if got := copied.RecordedRoot(); got != originalRoot {
		t.Fatalf("RecordedRoot() = %q, want %q", got, originalRoot)
	}
}

func TestMonorepoSubprojectIsNotTakenOverByTheRepoRoot(t *testing.T) {
	// Review finding: any directory of a repo whose main checkout had orai.toml
	// resolved to the root.
	tmp := resolvePath(t, t.TempDir())
	mono := filepath.Join(tmp, "mono")
	writeProject(t, mono, minimalConfig, true)
	sub := filepath.Join(mono, "sub")
	writeProject(t, sub, minimalConfig, false)
	if err := os.MkdirAll(filepath.Join(sub, "deeper"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := project.Locate(sub); err != nil || got != resolvePath(t, sub) {
		t.Fatalf("Locate(sub) = (%q, %v), want %q", got, err, resolvePath(t, sub))
	}
	if got, err := project.Locate(filepath.Join(sub, "deeper")); err != nil || got != resolvePath(t, sub) {
		t.Fatalf("Locate(sub/deeper) = (%q, %v), want %q", got, err, resolvePath(t, sub))
	}
	if got, err := project.Locate(mono); err != nil || got != resolvePath(t, mono) {
		t.Fatalf("Locate(mono) = (%q, %v), want %q", got, err, resolvePath(t, mono))
	}
}

func TestSubprojectInALinkedWorktreeMapsToTheSameSubprojectInMain(t *testing.T) {
	tmp := resolvePath(t, t.TempDir())
	main := filepath.Join(tmp, "repo")
	writeProject(t, main, minimalConfig, true)
	writeProject(t, filepath.Join(main, "app"), minimalConfig, false)
	git(t, main, "add", "-A")
	git(t, main, "commit", "-q", "-m", "init")
	worktree := filepath.Join(tmp, "repo-dev")
	git(t, main, "worktree", "add", "-q", "-b", "dev", worktree)

	if got, err := project.Locate(filepath.Join(worktree, "app")); err != nil || got != resolvePath(t, filepath.Join(main, "app")) {
		t.Fatalf("Locate(worktree/app) = (%q, %v), want %q", got, err, resolvePath(t, filepath.Join(main, "app")))
	}
	if got, err := project.Locate(worktree); err != nil || got != resolvePath(t, main) {
		t.Fatalf("Locate(worktree) = (%q, %v), want %q", got, err, resolvePath(t, main))
	}
}

// --- SetupTarget ----------------------------------------------------------------------

func TestSetupTargetPrefersThisRepositoryOverConfigsAboveIt(t *testing.T) {
	tmp := resolvePath(t, t.TempDir())
	outer := writeProject(t, filepath.Join(tmp, "outer"), minimalConfig, false) // someone else's project above
	repo := filepath.Join(outer.Root, "apps", "new")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-q", "-b", "trunk")
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := project.SetupTarget(filepath.Join(repo, "src")); got != resolvePath(t, repo) {
		t.Fatalf("SetupTarget(repo/src) = %q, want %q", got, resolvePath(t, repo))
	}
	plain := filepath.Join(outer.Root, "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := project.SetupTarget(plain); got != resolvePath(t, plain) {
		t.Fatalf("SetupTarget(plain) = %q, want %q", got, resolvePath(t, plain))
	}
	existing := writeProject(t, repo, minimalConfig, false) // once set up, subdirectories resolve to it
	if got := project.SetupTarget(filepath.Join(repo, "src")); got != existing.Root {
		t.Fatalf("SetupTarget(repo/src) after setup = %q, want %q", got, existing.Root)
	}
}
