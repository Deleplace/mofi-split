package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

var (
	filename   string
	count      int
	goroutine  int
	version    bool
	ver        string
	buffer     int
	parallel   bool
	linelength int
	VERSION    string = "v0.0.0"
)

func init() {
	flag.StringVar(&filename, "filename", "input.txt", "file name to split")
	flag.IntVar(&count, "count", 0, "split the file in these many files")
	flag.BoolVar(&version, "version", false, "show version")
	flag.IntVar(&buffer, "buffer", 1, "buffer size in MB")
	flag.IntVar(&linelength, "linelength", 17, "length of each line (length of each number + 1 for newline)")
	flag.BoolVar(&parallel, "parallel", false, "split the file in parallel (default false)")
}

func split(count, buffer int, filename, filenamePrefix string) error {
	f, err := os.OpenFile(filename, os.O_RDONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return err
	}
	fileSize := fi.Size()
	// each line is 20 bytes
	// so we can calculate the number of lines per chunk
	linesPerChunk := int((fileSize / int64(linelength)) / int64(count))
	// each chunk is 17 bytes per line (16 digits + 1 newline)
	// we need to do it this way to avoid any rounding errors
	// for example if we have say 100 lines. that is 1700 bytes
	// if we want to split it into 3 files. then each file should have 566 bytes
	// 566 bytes does not divide in 17 bytes per line.
	// instead if we calculate lines per chunk it comes to be 33
	// then reach chunk size is 660 bytes exactly
	chunkSize := linesPerChunk * linelength
	// buffer size is in MB
	bufferSize := buffer * 1024 * 1024
	if chunkSize < bufferSize {
		bufferSize = chunkSize
	}

	for i := 0; i < count; i++ {
		f.Seek(int64(chunkSize*i), 0)
		buf := make([]byte, bufferSize)
		file, err := os.OpenFile(fmt.Sprintf("%s_%04d.txt", filenamePrefix, i+1), os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		// for the last chunk we want to write whatever we have left into the last file.
		// this way no data is left.
		if i == count-1 {
			for {
				n, err := f.Read(buf)
				if err != nil {
					if err == io.EOF {
						break
					}
					return err
				}
				file.Write(buf[:n])
				buf = make([]byte, bufferSize)
			}
		} else {
			readSoFar := 0

			for readSoFar < int(chunkSize) {
				n, err := f.Read(buf)
				if err != nil {
					return err
				}
				file.Write(buf[:n])
				readSoFar += n
				buf = make([]byte, bufferSize)
			}
		}

		file.Close()
	}

	return nil
}

func splitParallel(count, goroutine, buffer int, filename, filenamePrefix string, ctx context.Context) error {
	f, err := os.OpenFile(filename, os.O_RDONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return err
	}
	fileSize := fi.Size()
	// see logic in split function
	linesPerChunk := int((fileSize / int64(linelength)) / int64(count))
	chunkSize := linesPerChunk * linelength
	bufferSize := buffer * 1024 * 1024
	if chunkSize < bufferSize {
		bufferSize = chunkSize
	}
	errs, _ := errgroup.WithContext(ctx)

	for i := 0; i < count; i++ {
		i := i
		errs.Go(func() error {
			source, err := os.OpenFile(filename, os.O_RDONLY, 0644)
			if err != nil {
				return err
			}
			destination, err := os.OpenFile(fmt.Sprintf("%s_%04d.txt", filenamePrefix, i+1), os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return nil
			}
			source.Seek(int64(chunkSize*i), 0)

			buf := make([]byte, bufferSize)
			if i == count-1 {
				for {
					n, err := source.Read(buf)
					if err != nil {
						if err == io.EOF {
							break
						}
						return err
					}
					destination.Write(buf[:n])
					buf = make([]byte, bufferSize)
				}
			} else {
				readSoFar := 0
				for readSoFar < int(chunkSize) {
					n, err := source.Read(buf)
					if err != nil {
						return err
					}
					destination.Write(buf[:n])
					readSoFar += n
					buf = make([]byte, bufferSize)
				}
			}

			source.Close()
			destination.Close()
			return nil

		})
	}
	return errs.Wait()
}

func duration(msg string, start time.Time) {
	fmt.Printf("%s took %s\n", msg, time.Since(start))
}

func main() {
	flag.Parse()

	if version {
		fmt.Println(ver)
		os.Exit(0)
	}

	if goroutine > count {
		goroutine = count
	}

	if count == 0 {
		fmt.Fprintln(os.Stderr, "count is required")
		flag.Usage()
		os.Exit(1)
	}

	if goroutine == 0 {
		goroutine = runtime.GOMAXPROCS(-1)
	}

	filenamePrefix := strings.Split(filename, ".")[0]

	defer duration("split", time.Now())

	if parallel {
		err := splitParallel(count, goroutine, buffer, filename, filenamePrefix, context.Background())
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	} else {
		err := split(count, buffer, filename, filenamePrefix)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
