package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	signatureupdater "github.com/Racherom/goAppleMailSignatureUpdater"
)

func main() {

	if len(os.Args) < 2 {
		panic(fmt.Sprintf("Pleas provide a signature file.\n Usage: %s [-w workdir] signaturePath ", os.Args[0]))
	}

	var workdir = "/tmp/signatures/"

	flag.StringVar(&workdir, "w", workdir, "Dir to temporarily store data. Will be created and removed afterward.")

	flag.Parse()
	found := false
	if err := signatureupdater.Update(workdir, func(signatureName string) io.Reader {
		if signatureName != os.Args[1] {
			return nil
		}
		found = true
		return os.Stdin
	}); err != nil {
		panic(err)
	}
	if !found {
		panic("Didn't found Signature. ")
	}
}
