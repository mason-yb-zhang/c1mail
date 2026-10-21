// 本地存储路径。
package main

import "os"

const defaultDataDir = "/storage/c1/local-apps/apps/mail/data"

func dataDir() string {
	if dir := os.Getenv("C1MAIL_DATA"); dir != "" {
		return dir
	}
	return defaultDataDir
}

func ensureDataDir() error {
	return os.MkdirAll(dataDir(), 0o755)
}
