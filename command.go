package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

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
	Region            string `json:"region"`
}

type Version struct {
	Ref string `json:"ref"`
}
type Input struct {
	Source  Source
	Version Version `json:"version"`
}
type Metadata struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type Output struct {
	Metadata []Metadata `json:"metadata"`
	Version  Version    `json:"version"`
}

type _OpenSSLCreds struct {
	key []byte
	iv  []byte
}

const defaultRegion = "us-east-1"

const openSSLSaltHeader = "Salted_" // OpenSSL salt is always this string + 8 bytes of actual salt

func main() {
	var i Input
	var o Output

	bytes, _ := ioutil.ReadAll(os.Stdin)
	err := json.Unmarshal(bytes, &i)
	if err != nil {
		o.Metadata = append(o.Metadata, Metadata{"error", err.Error()})
		PrintOut(&o)
		return
	}

	o.Version.Ref = i.Version.Ref

	auth, err := aws.GetAuth(i.Source.AccessKeyId, i.Source.SecretAccessKey, "", time.Time{})
	if err != nil {
		log.Fatalf("Authentication error: %s", err.Error())
	}

	// use a default region of us east if one wasn't provided to be
	// backwards compatible
	if i.Source.Region == "" {
		log.Print("No region specified, falling back to default region: " + defaultRegion)
		i.Source.Region = defaultRegion
	}

	r, ok := aws.Regions[i.Source.Region]
	if !ok {
		log.Fatal(i.Source.Region + " is not valid.")
	}

	connection := s3.New(auth, r)

	// Load the files in an array
	var files []File
	files = append(files, File{
		FilePath:   i.Source.SecretsFile,
		Passphrase: i.Source.SecretsPassphrase,
		OutputName: "secrets.yml",
	})
	files = append(files, File{
		FilePath:   i.Source.BoshCert,
		OutputName: "boshCA.crt",
	})

	// Check what the program name is and run the requested script
	_, file := filepath.Split(os.Args[0])
	switch file {
	case "in":
		RunIn(files, connection, &i, &o)
		PrintOut(&o)
	case "check":
		v := RunCheck(files, connection, &i)
		str := "[{\"ref\":\"" + strconv.Itoa(v) + "\"}]"
		os.Stdout.Write([]byte(str))
	}

}

// Run the `check` command
func RunCheck(files []File, connection *s3.S3, i *Input) int {
	headers := map[string][]string{}

	timeLayout := "Mon, 02 Jan 2006 15:04:05 GMT"

	lastDate := 0

	// Loop through all the files
	for _, f := range files {
		bucket := connection.Bucket(i.Source.BucketName)

		resp, _ := bucket.Head(f.FilePath, headers)
		t, _ := time.Parse(timeLayout, resp.Header.Get("Last-Modified"))
		if lastDate < int(t.Unix()) {
			lastDate = int(t.Unix())
		}
	}

	return lastDate
}

// Run the `in` command
func RunIn(files []File, connection *s3.S3, i *Input, o *Output) {

	baseDir := ""

	if len(os.Args) > 1 {
		baseDir = os.Args[1] + "/"
	}

	// Loop through all the files
	for _, f := range files {

		// Get the file from S3
		data, err := GetFile(connection, i.Source.BucketName, f.FilePath)
		if err != nil {
			o.Metadata = append(o.Metadata, Metadata{"error", err.Error()})
			return
		}

		// If it is encrypt it, decrypt it
		if f.Passphrase != "" {
			data, err = OpenSSLDecrypt(data, f.Passphrase)
			if err != nil {
				o.Metadata = append(o.Metadata, Metadata{"error", err.Error()})
				return
			}
		}

		// Write the file
		ioutil.WriteFile(baseDir+f.OutputName, data, 0644)
	}
	o.Metadata = append(o.Metadata, Metadata{"download", "complete"})
	o.Metadata = append(o.Metadata, Metadata{"dir", baseDir})
}

func GetFile(conn *s3.S3, bucket_name string, path string) ([]byte, error) {
	bucket := conn.Bucket(bucket_name)
	data, err := bucket.Get(path)

	return data, err
}

//OpenSSLDecrypt This OpenSSL AES-256-CBC decoding was taken from https://play.golang.org/p/r3VObSIB4o
func OpenSSLDecrypt(data []byte, passphrase string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return nil, err
	}
	saltHeader := data[:aes.BlockSize]
	if string(saltHeader[:7]) != openSSLSaltHeader {
		return nil, fmt.Errorf("Does not appear to have been encrypted with OpenSSL, salt header missing.")
	}
	salt := saltHeader[8:]
	creds, err := extractOpenSSLCreds([]byte(passphrase), salt)
	if err != nil {
		return nil, err
	}
	return decrypt(creds.key, creds.iv, data)
}

func decrypt(key, iv, data []byte) ([]byte, error) {
	if len(data) == 0 || len(data)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("bad blocksize(%v), aes.BlockSize = %v\n", len(data), aes.BlockSize)
	}
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	cbc := cipher.NewCBCDecrypter(c, iv)
	cbc.CryptBlocks(data[aes.BlockSize:], data[aes.BlockSize:])
	out, err := pkcs7Unpad(data[aes.BlockSize:], aes.BlockSize)
	if out == nil {
		return nil, err
	}
	return out, nil
}

// openSSLEvpBytesToKey follows the OpenSSL (undocumented?) convention for extracting the key and IV from passphrase.
// It uses the EVP_BytesToKey() method which is basically:
// D_i = HASH^count(D_(i-1) || password || salt) where || denotes concatentaion, until there are sufficient bytes available
// 48 bytes since we're expecting to handle AES-256, 32bytes for a key and 16bytes for the IV
func extractOpenSSLCreds(password, salt []byte) (_OpenSSLCreds, error) {
	m := make([]byte, 48)
	prev := []byte{}
	for i := 0; i < 3; i++ {
		prev = hash(prev, password, salt)
		copy(m[i*16:], prev)
	}
	return _OpenSSLCreds{key: m[:32], iv: m[32:]}, nil
}

func hash(prev, password, salt []byte) []byte {
	a := make([]byte, len(prev)+len(password)+len(salt))
	copy(a, prev)
	copy(a[len(prev):], password)
	copy(a[len(prev)+len(password):], salt)
	return md5sum(a)
}

func md5sum(data []byte) []byte {
	h := md5.New()
	h.Write(data)
	return h.Sum(nil)
}

// pkcs7Unpad returns slice of the original data without padding.
func pkcs7Unpad(data []byte, blocklen int) ([]byte, error) {
	if blocklen <= 0 {
		return nil, fmt.Errorf("invalid blocklen %d", blocklen)
	}
	if len(data)%blocklen != 0 || len(data) == 0 {
		return nil, fmt.Errorf("invalid data len %d", len(data))
	}
	padlen := int(data[len(data)-1])
	if padlen > blocklen || padlen == 0 {
		return nil, fmt.Errorf("invalid padding")
	}
	pad := data[len(data)-padlen:]
	for i := 0; i < padlen; i++ {
		if pad[i] != byte(padlen) {
			return nil, fmt.Errorf("invalid padding")
		}
	}
	return data[:len(data)-padlen], nil
}

func PrintOut(o *Output) {
	b, err := json.Marshal(o)
	if err != nil {
		fmt.Println("error:", err)
	}
	os.Stdout.Write(b)
}
