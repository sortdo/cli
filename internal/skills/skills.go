// Package skills installs the sortdo Agent Skill (SKILL.md) where coding agents read personal skills.
package skills

import (
	"os"
	"path/filepath"
)

// Name is the skill's folder name; it matches the name in SKILL.md.
const Name = "sortdo"

// DefaultDirs are the skills folders to install into: ~/.agents/skills, read by Codex, Cursor, GitHub Copilot,
// Gemini CLI and other agents, and ~/.claude/skills for Claude Code when ~/.claude exists.
func DefaultDirs(home string) []string {
	dirs := []string{filepath.Join(home, ".agents", "skills")}
	if info, err := os.Stat(filepath.Join(home, ".claude")); err == nil && info.IsDir() {
		dirs = append(dirs, filepath.Join(home, ".claude", "skills"))
	}
	return dirs
}

// Install writes SKILL.md into <dir>/sortdo for each skills folder and returns the files it wrote.
func Install(skill []byte, dirs []string) ([]string, error) {
	var written []string
	for _, dir := range dirs {
		folder := filepath.Join(dir, Name)
		if err := os.MkdirAll(folder, 0o755); err != nil {
			return written, err
		}
		file := filepath.Join(folder, "SKILL.md")
		if err := os.WriteFile(file, skill, 0o644); err != nil {
			return written, err
		}
		written = append(written, file)
	}
	return written, nil
}

// Uninstall removes <dir>/sortdo from each skills folder and returns the folders it removed.
func Uninstall(dirs []string) ([]string, error) {
	var removed []string
	for _, dir := range dirs {
		folder := filepath.Join(dir, Name)
		if _, err := os.Stat(filepath.Join(folder, "SKILL.md")); err != nil {
			continue
		}
		if err := os.RemoveAll(folder); err != nil {
			return removed, err
		}
		removed = append(removed, folder)
	}
	return removed, nil
}
