// Файл: internal/instance/lock.go
package instance

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Lock берёт эксклюзивную блокировку файла: одна копия программы на папку данных.
// ok=false — блокировку держит другая копия. release снимает блокировку и закрывает файл.
// Берётся до открытия базы, поэтому две копии не откроют базу и не поднимут одни и те же задания.
func Lock(path string) (release func() error, ok bool, err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, false, fmt.Errorf("папка блокировки: %w", err)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, false, fmt.Errorf("файл блокировки: %w", err)
	}
	locked, err := tryLock(f)
	if err != nil || !locked {
		return nil, false, errors.Join(err, f.Close())
	}
	return func() error { return errors.Join(unlock(f), f.Close()) }, true, nil
}
