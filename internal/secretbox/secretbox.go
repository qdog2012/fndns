package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const keySize = 32

type Box struct {
	aead cipher.AEAD
}

func Open(keyPath string) (*Box, error) {
	key, err := loadOrCreateKey(keyPath)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建凭据加密器: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建 GCM 加密器: %w", err)
	}
	return &Box{aead: aead}, nil
}

func loadOrCreateKey(path string) ([]byte, error) {
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != keySize {
			return nil, fmt.Errorf("主密钥长度无效: %d", len(key))
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("读取主密钥: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("创建数据目录: %w", err)
	}
	key = make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("生成主密钥: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return loadOrCreateKey(path)
		}
		return nil, fmt.Errorf("保存主密钥: %w", err)
	}
	if _, err := file.Write(key); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("写入主密钥: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("关闭主密钥文件: %w", err)
	}
	return key, nil
}

func (b *Box) Seal(value any) ([]byte, error) {
	plain, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("编码凭据: %w", err)
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("生成随机数: %w", err)
	}
	return b.aead.Seal(nonce, nonce, plain, []byte("fndns-credential-v1")), nil
}

func (b *Box) Open(ciphertext []byte, target any) error {
	if len(ciphertext) < b.aead.NonceSize() {
		return errors.New("加密凭据数据不完整")
	}
	nonce := ciphertext[:b.aead.NonceSize()]
	plain, err := b.aead.Open(nil, nonce, ciphertext[b.aead.NonceSize():], []byte("fndns-credential-v1"))
	if err != nil {
		return errors.New("无法解密凭据，请确认主密钥未被替换")
	}
	if err := json.Unmarshal(plain, target); err != nil {
		return fmt.Errorf("解析凭据: %w", err)
	}
	return nil
}
