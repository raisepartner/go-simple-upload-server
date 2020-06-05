package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func unarchive(path string, dstDir string) (kind string, err error) {
	if strings.HasSuffix(path, "zip") {
		return "zip", unzip(path, dstDir)
	} else if strings.HasSuffix(path, "tar") {
		file, err := os.Open(path)
		if err != nil {
			logger.WithError(err).Error(fmt.Errorf("cannot read from: %s", path))
			return "tar", err
		}
		defer func() {
			if err := file.Close(); err != nil { panic(err) }
		}()
		return "tar", untar(file, dstDir)
	} else if strings.HasSuffix(path, "tar.gz") || strings.HasSuffix(path, "tgz") {
		file, err := os.Open(path)
		if err != nil {
			logger.WithError(err).Error(fmt.Errorf("cannot read from: %s", path))
			return "tgz", err
		}
		tarf, err := gzip.NewReader(file)
		if err != nil {
			logger.WithError(err).Error(fmt.Errorf("unarchive: failed to create un-gzip : %s", path))
			return "tgz", err
		}
		return "tgz", untar(tarf, dstDir)
	}
	return "file", nil
}

func unzip(src string, dest string) error {
	// based on https://stackoverflow.com/questions/20357223/easy-way-to-unzip-file-with-golang
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Close(); err != nil {
			panic(err)
		}
	}()

	err = os.MkdirAll(dest, 0755)
	if err != nil {
		logger.WithError(err).Error("failed to create the unzip destination dir")
		return err
	}

	// Closure to address file descriptors issue with all the deferred .Close() methods
	extractAndWriteFile := func(f *zip.File) error {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer func() {
			if err := rc.Close(); err != nil {
				panic(err)
			}
		}()

		path := filepath.Join(dest, f.Name)

		if f.FileInfo().IsDir() {
			err := os.MkdirAll(path, f.Mode())
			if err != nil {
				logger.WithError(err).Error(fmt.Errorf("unzip: failed to create sub directory: %s", path))
				return err
			}
		} else {
			err := os.MkdirAll(filepath.Dir(path), f.Mode())
			if err != nil {
				logger.WithError(err).Error(fmt.Errorf("unzip: failed to create parent directory for: %s", path))
				return err
			}
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer func() {
				if err := f.Close(); err != nil {
					panic(err)
				}
			}()

			_, err = io.Copy(f, rc)
			if err != nil {
				return err
			}
		}
		return nil
	}

	for _, f := range r.File {
		err := extractAndWriteFile(f)
		if err != nil {
			return err
		}
	}

	return nil
}

func untar(r io.Reader, dst string) error {

	//gzr, err := gzip.NewReader(r)
	//if err != nil {
	//	return err
	//}
	//defer gzr.Close()

	tr := tar.NewReader(r)

	for {
		header, err := tr.Next()

		switch {

		// if no more files are found return
		case err == io.EOF:
			return nil

		// return any other error
		case err != nil:
			return err

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		// the target location where the dir/file should be created
		target := filepath.Join(dst, header.Name)

		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()

		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				return err
			}

			// manually close here after each file operation; defering would cause each file close
			// to wait until all operations have completed.
			f.Close()
		}
	}
}