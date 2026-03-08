package javacompat

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type Oracle struct {
	repoRoot string
	buildDir string
	javaExe  string
}

var (
	prepareOnce sync.Once
	prepared    *Oracle
	prepareErr  error
)

func Prepare() (*Oracle, error) {
	prepareOnce.Do(func() {
		prepared, prepareErr = prepareOracle()
	})
	return prepared, prepareErr
}

func prepareOracle() (*Oracle, error) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		return nil, err
	}
	repoRoot = normalizePath(repoRoot)

	javaExe, err := exec.LookPath("java")
	if err != nil {
		return nil, err
	}
	javacExe, err := exec.LookPath("javac")
	if err != nil {
		return nil, err
	}

	buildDir, err := os.MkdirTemp("", "hath-java-oracle-")
	if err != nil {
		return nil, err
	}
	buildDir = normalizePath(buildDir)

	javaFiles, err := filepath.Glob(filepath.Join(repoRoot, "HentaiAtHome_1.6.4_src", "src", "hath", "base", "*.java"))
	if err != nil {
		return nil, err
	}
	if len(javaFiles) == 0 {
		return nil, errors.New("java source files not found")
	}

	helperFile := filepath.Join(repoRoot, "testdata", "java_oracle", "src", "hath", "base", "ProtocolOracle.java")
	if _, err := os.Stat(helperFile); err != nil {
		return nil, err
	}

	args := []string{"-encoding", "UTF-8", "-source", "8", "-target", "8", "-d", buildDir}
	args = append(args, javaFiles...)
	args = append(args, helperFile)

	cmd := exec.Command(javacExe, args...)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("javac failed: %w\n%s", err, string(output))
	}

	return &Oracle{repoRoot: repoRoot, buildDir: buildDir, javaExe: javaExe}, nil
}

func (o *Oracle) Run(args ...string) (map[string]string, error) {
	cmdArgs := append([]string{"-cp", o.buildDir, "hath.base.ProtocolOracle"}, args...)
	cmd := exec.Command(o.javaExe, cmdArgs...)
	cmd.Dir = o.repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("java oracle failed: %w\n%s", err, string(output))
	}

	result := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		idx := strings.Index(line, "ORACLE\t")
		if idx < 0 {
			continue
		}
		line = line[idx:]

		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}

		result[parts[1]] = unescape(parts[2])
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("java oracle returned no parseable output\n%s", string(output))
	}

	return result, nil
}

func normalizePath(path string) string {
	if runtime.GOOS == "windows" && strings.HasPrefix(path, `\\?\`) {
		return strings.TrimPrefix(path, `\\?\`)
	}
	return path
}

func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	wd = normalizePath(wd)

	for current := wd; current != filepath.Dir(current); current = filepath.Dir(current) {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(current, "HentaiAtHome_1.6.4_src")); err == nil {
				return current, nil
			}
		}
	}

	return "", errors.New("repository root not found")
}

func unescape(value string) string {
	replacer := strings.NewReplacer(`\\`, `\`, `\n`, "\n", `\r`, "\r", `\t`, "\t")
	return replacer.Replace(value)
}
