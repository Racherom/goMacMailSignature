package signatureupdater

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/go-multierror"

	"github.com/groob/plist"
)

// Update ...
func Update(workdir string, callback func(string) io.Reader) error {
	if callback == nil {
		return fmt.Errorf("Callback should no be empty. ")
	}

	signaturesPath, err := getSignaturesPath()
	if err != nil {
		return fmt.Errorf("Error could not get signaturesPath: %v", err)
	}

	if err := os.Mkdir(workdir, 0777); err != nil && !os.IsExist(err) {
		return fmt.Errorf("Error create workdir: %v", err)
	}
	defer clean(workdir)

	if err := runApplescript(`quit app "Mail"`, nil); err != nil {
		return fmt.Errorf("Error stop mail: %v", err)
	}

	if err := copyFile(signaturesPath+"AllSignatures.plist", workdir); err != nil {
		return fmt.Errorf("Error copy AllSignatures.plist: %v", err)
	}

	allSignaturesfile, err := os.Open(workdir + "/AllSignatures.plist")
	if err != nil {
		return fmt.Errorf("Error open AllSignatures.plist: %v", err)
	}
	plistDecoder := plist.NewDecoder(allSignaturesfile)

	var allSignatures []struct {
		SignatureName     string `plist:"SignatureName"`
		SignatureUniqueID string `plist:"SignatureUniqueId"`
	}

	if plistDecoder.Decode(&allSignatures); err != nil {
		return fmt.Errorf("Could not decode AllSignatures.plist: %v", err)
	}

	var updateErrors error
	for _, signature := range allSignatures {
		newSignature := callback(signature.SignatureName)
		if newSignature == nil {
			continue
		}

		if err := updateSignature(workdir, signaturesPath, signature.SignatureUniqueID, newSignature); err != nil {
			updateErrors = multierror.Append(updateErrors, err)
		}
	}

	if err := runApplescript(`activate app "Mail"`, nil); err != nil {
		updateErrors = multierror.Append(updateErrors, fmt.Errorf("Error start mail: %v", err))
	}

	return updateErrors
}

func updateSignature(workdir, signaturesPath, signatureID string, newSignature io.Reader) error {
	signatureFile := fmt.Sprintf("%s.mailsignature", signatureID)
	signatureFileWorkPath := path.Join(workdir, signatureFile)

	if err := copyFile(signaturesPath+signatureFile, workdir); err != nil {
		return fmt.Errorf("Error copy %s: %v", signatureFile, err)
	}

	if err := runCommand(nil, "chflags", "nouchg", signatureFileWorkPath); err != nil {
		return fmt.Errorf("Error unlook %s: %v", signatureFile, err)
	}

	if err := updateSignatureFile(signatureFileWorkPath, newSignature); err != nil {
		return err
	}

	if err := runCommand(nil, "chflags", "uchg", signatureFileWorkPath); err != nil {
		return fmt.Errorf("Error look %s: %v", signatureFile, err)
	}
	defer runCommand(nil, "chflags", "nouchg", signatureFileWorkPath)

	if err := copyFile(signatureFileWorkPath, signaturesPath); err != nil {
		return fmt.Errorf("Error copy %s back: %v", signatureFile, err)
	}

	return nil
}

func updateSignatureFile(signatureFile string, newSignature io.Reader) error {
	f, err := os.OpenFile(signatureFile, os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	fileHeader := bytes.NewBuffer(nil)
	count := 0
	s := bufio.NewScanner(f)
	for s.Scan() {
		fileHeader.Write(s.Bytes())
		fileHeader.WriteString("\n")
		count++
		if count > 5 {
			break
		}
	}

	if err := s.Err(); err != nil {
		return fmt.Errorf("Error scan signatur %s: %v", signatureFile, err)
	}

	f.Truncate(0)
	f.Seek(0, 0)

	if _, err = fileHeader.WriteTo(f); err != nil {
		return fmt.Errorf("Error write header to signatur file %s: %v", signatureFile, err)
	}

	if _, err = io.Copy(f, newSignature); err != nil {
		return fmt.Errorf("Error write new signatur to %s: %v", signatureFile, err)
	}

	return nil
}

func clean(workdir string) error {
	return os.RemoveAll(workdir)
}
func runCommand(out io.Writer, name string, arg ...string) error {
	cmd := exec.Command(name, arg...)
	cmd.Stdout = out
	return cmd.Run()
}

func runApplescript(command string, out io.Writer) error {
	return runCommand(out, "osascript", "-e", command)
}

func copyFile(from, to string) error {
	return runApplescript(fmt.Sprintf(`tell application "Finder" to duplicate POSIX file "%s" to POSIX file "%s" replacing yes`, from, to), nil)
}

func getSignaturesPath() (string, error) {
	out := bytes.NewBuffer(nil)
	if err := runApplescript(fmt.Sprintf(`tell app "Finder" to get name of folders of folder POSIX file "%s"`, os.Getenv("HOME")+"/Library/Mail/"), out); err != nil {
		return "", fmt.Errorf("Error get mail folder content: %v", err)
	}

	versionNumber := 0
	versionFolderRegex := regexp.MustCompile("^V([0-9])$")
	folders := strings.Split(out.String(), ", ")
	for _, folder := range folders {
		if found := versionFolderRegex.FindStringSubmatch(strings.Trim(folder, " \n")); len(found) == 2 {
			version, err := strconv.Atoi(found[1])
			if err != nil {
				log.Println(err)
				continue
			}
			if version > versionNumber {
				versionNumber = version
			}
		}
	}

	if versionNumber == 0 {
		return "", fmt.Errorf("Error could not find mail version folder")
	}
	return fmt.Sprintf("%s/Library/Mail/V%d/MailData/Signatures/", os.Getenv("HOME"), versionNumber), nil
}
