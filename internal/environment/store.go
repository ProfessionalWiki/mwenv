package environment

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

var (
	ErrNotFound = errors.New("environment not found")
	ErrBusy     = errors.New("another mwenv command is working on this environment")
)

type Env struct {
	ID         string `json:"id"`
	Worktree   string `json:"worktree"`
	Type       string `json:"type"`
	Name       string `json:"name"`
	MediaWiki  string `json:"mediawiki"`
	PHP        string `json:"php"`
	Database   string `json:"database"`
	Port       int    `json:"port"`
	CoreCommit string `json:"coreCommit"`

	// Created is false until every creation step has succeeded.
	Created bool `json:"created"`
}

func (e Env) ComposeProject() string {
	return "mwenv-" + e.ID
}

type Store struct {
	home string
}

func NewStore(home string) Store {
	return Store{home: home}
}

func (s Store) dir(id string) string {
	return filepath.Join(s.home, "envs", id)
}

func (s Store) stateFile(id string) string {
	return filepath.Join(s.dir(id), "env.json")
}

func (s Store) lockFile(id string) string {
	return filepath.Join(s.dir(id), "lock")
}

func (s Store) ComposeFile(id string) string {
	return filepath.Join(s.dir(id), "compose.json")
}

func (s Store) Save(env Env) error {
	content, err := json.MarshalIndent(env, "", "\t")
	if err != nil {
		return err
	}

	return writeAtomically(s.stateFile(env.ID), append(content, '\n'))
}

func (s Store) SaveComposeFile(id string, content []byte) error {
	return writeAtomically(s.ComposeFile(id), content)
}

func writeAtomically(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, content, 0o644); err != nil {
		return err
	}

	return os.Rename(temporary, path)
}

func (s Store) Load(id string) (Env, error) {
	content, err := os.ReadFile(s.stateFile(id))
	if errors.Is(err, fs.ErrNotExist) {
		return Env{}, ErrNotFound
	}
	if err != nil {
		return Env{}, err
	}

	var env Env
	if err := json.Unmarshal(content, &env); err != nil {
		return Env{}, fmt.Errorf("cannot parse the state of %s: %w", id, err)
	}

	return env, nil
}

// List returns the environments sorted by ID.
func (s Store) List() ([]Env, error) {
	entries, err := os.ReadDir(filepath.Join(s.home, "envs"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var envs []Env
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		env, err := s.Load(entry.Name())
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		envs = append(envs, env)
	}

	return envs, nil
}

func (s Store) Remove(id string) error {
	return os.RemoveAll(s.dir(id))
}

type Lock struct {
	file *os.File
}

func (l *Lock) Release() {
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	_ = l.file.Close()
}

// TryLock takes the lock of one environment, or fails with ErrBusy when another process holds it.
func (s Store) TryLock(id string) (*Lock, error) {
	lock, err := takeLock(s.lockFile(id), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, ErrBusy) {
		return nil, fmt.Errorf("%s: %w", id, err)
	}
	return lock, err
}

// IsLocked reports whether another process holds the lock of an environment, without creating the lock.
func (s Store) IsLocked(id string) bool {
	file, err := os.OpenFile(s.lockFile(id), os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer file.Close()

	return errors.Is(syscall.Flock(int(file.Fd()), syscall.LOCK_SH|syscall.LOCK_NB), syscall.EWOULDBLOCK)
}

// LockPorts takes the lock that keeps parallel starts from picking the same port, waiting while another process
// holds it.
func (s Store) LockPorts() (*Lock, error) {
	return takeLock(filepath.Join(s.home, "lock"), syscall.LOCK_EX)
}

func takeLock(path string, how int) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}

	err = syscall.Flock(int(file.Fd()), how)
	if err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrBusy
		}
		return nil, err
	}

	// destroy removes the lock file with its directory, so a lock taken on the removed file protects nothing.
	lock := &Lock{file: file}
	if !isLinked(path, file) {
		lock.Release()
		return nil, ErrBusy
	}

	return lock, nil
}

func isLinked(path string, file *os.File) bool {
	pathInfo, err := os.Stat(path)
	if err != nil {
		return false
	}
	fileInfo, err := file.Stat()
	return err == nil && os.SameFile(pathInfo, fileInfo)
}
