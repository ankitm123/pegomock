package remove

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"
)

func Remove(
	path string,
	recursive bool,
	shouldConfirm bool,
	dryRun bool,
	silent bool,
	out io.Writer,
	in io.Reader,
	removeFn func(path string) error,
) {
	filepaths, matchersDirPaths, e := getFilePaths(recursive, path, out)
	if e != nil {
		fmt.Fprintln(out, e.Error())
		return
	}
	if len(filepaths) == 0 {
		fmt.Fprintln(out, "No files to remove.")
		return
	}

	allFilepathsSorted := make([]string, len(filepaths))
	copy(allFilepathsSorted, filepaths)
	for matchersDirPath := range matchersDirPaths {
		if containsOnlyGeneratedFiles(matchersDirPath, allFilepathsSorted) {
			allFilepathsSorted = append(allFilepathsSorted, matchersDirPath)
		}
	}
	sort.Strings(allFilepathsSorted)

	if dryRun {
		fmt.Fprintln(out, "This is a dry-run. Would delete the following files:")
		fmt.Fprintln(out, strings.Join(allFilepathsSorted, "\n"))
		return
	}

	if shouldConfirm {
		fmt.Fprintln(out, "Will delete the following files:")
		fmt.Fprintln(out, strings.Join(allFilepathsSorted, "\n"))
		if !askForConfirmation("Continue?", in, out) {
			return
		}
	} else if !silent {
		fmt.Fprintln(out, "Deleting the following files:")
		fmt.Fprintln(out, strings.Join(allFilepathsSorted, "\n"))
	}

	var errs []error
	for _, filepath := range filepaths {
		e := removeFn(filepath)
		if e != nil {
			errs = append(errs, e)
		}
	}
	for matcherPath := range matchersDirPaths {
		if dirEmpty(matcherPath) {
			e := removeFn(matcherPath)
			if e != nil {
				errs = append(errs, e)
			}
		}
	}
	if len(errs) > 0 {
		fmt.Fprintf(out, "There were some errors when trying to delete files: %v", errs)
	}
}
func containsOnlyGeneratedFiles(matchersDirPath string, generatedFilenames []string) bool {
	f, e := os.Open(matchersDirPath)
	if e != nil {
		return false
	}
	defer f.Close()
	filenamesInMatchersDir, e := f.Readdirnames(-1)
	if e != nil {
		return false
	}
	if len(difference(matchersDirPath, filenamesInMatchersDir, generatedFilenames)) == 0 {
		return true
	}
	return false
}

func difference(root string, a, b []string) map[string]bool {
	m := make(map[string]bool, len(b))
	for _, s := range a {
		m[filepath.Join(root, s)] = true
	}
	for _, s := range b {
		delete(m, s)
	}
	return m
}

func dirEmpty(path string) bool {
	f, e := os.Open(path)
	if e != nil {
		return false
	}
	defer f.Close()

	_, e = f.Readdirnames(1)
	return e == io.EOF
}

func getFilePaths(recursive bool, path string, out io.Writer) ([]string, map[string]bool, error) {
	matcherPaths := make(map[string]bool)
	var walk func(root string, walkFn filepath.WalkFunc) error
	if recursive {
		walk = filepath.Walk
	} else {
		walk = walkFilesInDir
	}
	filepaths := make([]string, 0)
	e := walk(path, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() && filepath.Ext(path) == ".go" && isPegomockGenerated(path, out) {
			filepaths = append(filepaths, path)
			if filepath.Base(filepath.Dir(path)) == "matchers" {
				matcherPaths[filepath.Dir(path)] = true
			}
		}
		return nil
	})
	if e != nil {
		return nil, nil, errors.New("Could not get files in path " + path)
	}
	return filepaths, matcherPaths, nil
}

func walkFilesInDir(path string, walk filepath.WalkFunc) error {
	fileInfos, e := ioutil.ReadDir(path)
	if e != nil {
		errors.New("Could not get files in path " + path)
	}
	for _, info := range fileInfos {
		walk(filepath.Join(path, info.Name()), info, nil)
	}
	return nil
}

func isPegomockGenerated(path string, out io.Writer) bool {
	file, e := os.Open(path)
	if e != nil {
		fmt.Fprintf(out, "Could not open file %v. Error: %v\n", path, e)
		return false
	}
	b := make([]byte, 50)
	_, e = file.Read(b)
	if e != nil {
		fmt.Fprintf(out, "Could not read from file %v. Error: %v\n", path, e)
		return false
	}
	if strings.Contains(string(b), "// Code generated by pegomock. DO NOT EDIT.") {
		return true
	}
	return false
}

func askForConfirmation(s string, in io.Reader, out io.Writer) bool {
	reader := bufio.NewReader(in)

	for {
		fmt.Fprintf(out, "%s [y/n]: ", s)

		response, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintln(out, "Could not get confirmation from StdIn", err)
			return false
		}

		response = strings.ToLower(strings.TrimSpace(response))

		if response == "y" || response == "yes" {
			return true
		} else if response == "n" || response == "no" {
			return false
		}
	}
}
