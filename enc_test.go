package arcsek

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/secure-io/sio-go"
)

func genKey(pw string) []byte {
	s := sha1.Sum([]byte(pw))
	return s[:16]
}

// Test we can delete the source of the enc reader
// on close

// Exists reports whether the named file or directory exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// Tests the Close method of the vault
func testNormalClose(t *testing.T) {
	tmpFile, err := ioutil.TempFile(".", "*.tar.gz")
	if err != nil {
		t.Fatal(err)
	}

	tmpPath := tmpFile.Name()

	// It doesn't matter if we have an enc reader or not.
	// We are testing delete on close
	v := VaultReader{nil, tmpFile, nil}
	if !fileExists(tmpPath) {
		t.Fatal("The file was not created")
	}

	// The file exists at this point
	if err = v.Close(); err != nil {
		// Something happened
		t.Fatal(err)
	}

	// at this point the file should NOT exist
	if fileExists(tmpPath) {
		t.Fatalf("The file '%s' was not deleted on close", tmpPath)
	}

	t.Logf("The file %s was deleted", tmpPath)
}

func TestVaultReader_Close(t *testing.T) {
	// Do 5 random open and close
	for i := 0; i < 5; i++ {
		t.Run(fmt.Sprintf("Normal Close() %dth", i+1), testNormalClose)
	}

	// Now remove the file WHILE still in use to test the error
	tmpFile, err := ioutil.TempFile(".", "*.tar.gz")
	if err != nil {
		t.Fatal(err)
	}

	tmpPath := tmpFile.Name()

	// It doesn't matter if we have an enc reader or not.
	// We are testing delete on close
	v := VaultReader{nil, tmpFile, nil}
	if !fileExists(tmpPath) {
		t.Fatal("The file was not created")
	}

	// The file exists at this point
	// delete it before
	if err = v.tmpFile.Close(); err != nil {
		t.Fatal(err)
	}

	if err = os.Remove(tmpPath); err != nil {
		t.Fatal(err)
	}

	if err = v.Close(); err == nil {
		// The err was not launched
		t.Fatal("No error launched for previously deleted file")
	}
}

// Test if the GCM can be created with a good or a bad key

func TestMakeGCMFromKey(t *testing.T) {
	tests := []struct {
		name string
		key  []byte
		good bool
	}{
		{"Good 128 bit (16 byte) key", []byte("0123456789ABCDEF"), true},
		{"Bad key with len 4 bytes (16 bit)", []byte("1234"), false},
		{"Good 256 bit (32 byte) key",
			[]byte("0123456789ABCDEF0123456789ABCDEF"), true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := createAESGCMFromKey(tc.key)
			if (tc.good && err != nil) || (!tc.good && err == nil) {
				t.Fatal("Error not corresponing to key")
			}
		})
	}
}

type vaultTC struct {
	name  string
	files []string
	key   []byte
	good  bool
}

func testMakeVaultGood(t *testing.T, tc vaultTC) {
	if _, err := NewVaultReader(tc.files, tc.key); err != nil {
		t.Fatal("Error should be nil in a good test case. Instead got ", err)
	}
}

func testMakeVaultBad(t *testing.T, tc vaultTC) {
	if _, err := NewVaultReader(tc.files, tc.key); err == nil {
		t.Fatal("Error should not be nil in a bad test case")
	}
}

func TestCreateVaultReader(t *testing.T) {
	goodFiles, _ := lsDir("testing-files/in")
	otherGoodFiles, _ := lsDir("testing-files/in/existance")
	tests := []vaultTC{
		{
			"Good files good key",
			goodFiles,
			genKey("password123"),
			true,
		},
		{
			"Good files bad key",
			goodFiles,
			[]byte("123"),
			false,
		},
		{
			"Bad files good key",
			[]string{"imaginary", "files"},
			genKey("123"),
			false,
		},
		{
			"Bad files Bad key",
			[]string{"imaginary", "files"},
			[]byte("BadKey"),
			false,
		},
		{
			"Another group of good files",
			otherGoodFiles,
			genKey("GoodBoi"),
			true,
		},
	}
	for _, test := range tests {
		// If the test case is good test a good result,
		// otherwise test a bad result
		t.Run(test.name, func(t *testing.T) {
			if test.good {
				// Everything is good and test it as such
				testMakeVaultGood(t, test)
			} else {
				// Something is bad and is supposed to fail
				testMakeVaultBad(t, test)
			}
		})
	}
}

func TestEncDec(t *testing.T) {
	// Create a vault
	files, _ := lsDir("testing-files/in")
	k := genKey("xdxdxdxd")
	vr, err := NewVaultReader(files, k)

	if err != nil {
		t.Fatal(err)
	}

	// now we are supposed to make a stream cipher to decrypt
	// it

	aes128gcm, _ := createAESGCMFromKey(k)
	s := sio.NewStream(aes128gcm, sio.BufSize)
	dr := s.DecryptReader(vr, vr.Nonce, nil)

	// copy the decrypted version
	tmp, err := ioutil.TempFile("testing-files/out", "dec-*-large.tar.gz")
	if err != nil {
		t.Fatal(err)
	}

	if _, err = io.Copy(tmp, dr); err != nil {
		t.Fatal(err)
	}
}

func TestNewTarReader(t *testing.T) {
	// Generate a very small encrypted archive
	files := []string{
		"testing-files/in/existance/testfile1.txt",
		"testing-files/in/existance/testfile2.txt",
	}

	k := genKey("Klara:3")

	vault, err := NewVaultReader(files, k)
	if err != nil {
		t.Fatal(err)
	}

	// Since we are only using less than a MB we can just
	// put everything in memory
	buff := bytes.NewBuffer(make([]byte, 0, 20))
	buff.Write(vault.Nonce)

	// This emulates an output file, we can now copy the enc data
	vault.WriteTo(buff)

	// Using this ficticious file we can decrypt it like it
	// was a file
	_, err = NewTarReaderNonce(buff, k)
	if err != nil {
		t.Fatal(err)
	}
}
