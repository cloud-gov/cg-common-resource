package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/goamz/goamz/aws"
	"github.com/goamz/goamz/s3"
)

type File struct {
	FilePath   string `json:"file_path"`
	Passphrase string
	OutputName string `json:"output_name"`
}

type Source struct {
	BucketName        string `json:"bucket_name"`
	AccessKeyId       string `json:"access_key_id"`
	SecretAccessKey   string `json:"secret_access_key"`
	SecretsFile       string `json:"secrets_file"`
	SecretsPassphrase string `json:"secrets_passphrase"`
	BoshCert          string `json:"bosh_cert"`
}

type Input struct {
	Source  Source
	Version struct {
		Ref string `json:"ref"`
	} `json:"version"`
}
type Metadata struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type Output struct {
	Metadata []Metadata `json:"metadata"`
	Version  struct {
		Ref string `json:"ref"`
	} `json:"version"`
}

func main() {
	var i Input
	var o Output

	baseDir := ""

	if len(os.Args) > 1 {
		baseDir = os.Args[1] + "/"
	}

	bytes, _ := ioutil.ReadAll(os.Stdin)
	err := json.Unmarshal(bytes, &i)
	if err != nil {
		o.Metadata = append(o.Metadata, Metadata{"error", err.Error()})
		PrintOut(&o)
		return
	}

	o.Version.Ref = i.Version.Ref

	auth := aws.Auth{
		AccessKey: i.Source.AccessKeyId,
		SecretKey: i.Source.SecretAccessKey,
	}

	connection := s3.New(auth, aws.USEast)

	// Load the files in an array
	var files []File
	files = append(files, File{
		FilePath:   i.Source.SecretsFile,
		Passphrase: i.Source.SecretsPassphrase,
		OutputName: baseDir + "secrets.yml",
	})
	files = append(files, File{
		FilePath:   i.Source.BoshCert,
		OutputName: baseDir + "boshCA.crt",
	})

	// Loop through all the files
	for _, f := range files {

		// Get the file from S3
		data, err := GetFile(connection, i.Source.BucketName, f.FilePath)
		if err != nil {
			o.Metadata = append(o.Metadata, Metadata{"error", err.Error()})
			PrintOut(&o)
			return
		}

		// If it is encrypt it, decrypt it
		if f.Passphrase != "" {
			data, err = Decrypt(data, f.Passphrase)
			if err != nil {
				o.Metadata = append(o.Metadata, Metadata{"error", err.Error()})
				PrintOut(&o)
				return
			}
		}

		// Write the file
		ioutil.WriteFile(f.OutputName, data, 0644)
	}
	o.Metadata = append(o.Metadata, Metadata{"download", "complete"})
	o.Metadata = append(o.Metadata, Metadata{"dir", baseDir})

	PrintOut(&o)

}

func GetFile(conn *s3.S3, bucket_name string, path string) ([]byte, error) {
	bucket := conn.Bucket(bucket_name)
	data, err := bucket.Get(path)

	return data, err
}

func Decrypt(data []byte, passphrase string) ([]byte, error) {
	cmd := exec.Command("openssl",
		"enc",
		"-aes-256-cbc",
		"-d",
		"-a",
		"-pass",
		"pass:"+passphrase,
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return []byte(""), err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return []byte(""), err
	}

	err = cmd.Start()
	if err != nil {
		return []byte(""), err
	}

	_, err = stdin.Write(data)
	if err != nil {
		return []byte(""), err
	}
	stdin.Close()

	buf := new(bytes.Buffer)
	buf.ReadFrom(stdout)

	cmd.Wait()

	return buf.Bytes(), nil
}

func PrintOut(o *Output) {
	b, err := json.Marshal(o)
	if err != nil {
		fmt.Println("error:", err)
	}
	os.Stdout.Write(b)
}
