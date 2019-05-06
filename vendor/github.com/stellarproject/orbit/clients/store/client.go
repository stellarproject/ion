/*
	Copyright (c) 2019 Stellar Project

	Permission is hereby granted, free of charge, to any person
	obtaining a copy of this software and associated documentation
	files (the "Software"), to deal in the Software without
	restriction, including without limitation the rights to use, copy,
	modify, merge, publish, distribute, sublicense, and/or sell copies
	of the Software, and to permit persons to whom the Software is
	furnished to do so, subject to the following conditions:

	The above copyright notice and this permission notice shall be
	included in all copies or substantial portions of the Software.

	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
	EXPRESS OR IMPLIED,
	INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
	FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
	IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
	HOLDERS BE LIABLE FOR ANY CLAIM,
	DAMAGES OR OTHER LIABILITY,
	WHETHER IN AN ACTION OF CONTRACT,
	TORT OR OTHERWISE,
	ARISING FROM, OUT OF OR IN CONNECTION WITH
	THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

package store

import (
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	Address     = "127.0.0.1:9300"
	MasterKey   = "stellarproject.io/master"
	StorePlugin = "stellarproject.io.store"
)

func New(myAddress string) (*Store, error) {
	slave := redis.NewPool(func() (redis.Conn, error) {
		return redis.Dial("tcp", Address)
	}, 5)
	conn := slave.Get()

	st := &Store{
		m: slave,
		s: slave,
	}
	master, err := redis.String(conn.Do("GET", MasterKey))
	conn.Close()
	if err != nil {
		if err == redis.ErrNil {
			// if there is no master, we are the master
			if myAddress != "" {
				conn = slave.Get()
				if _, err := conn.Do("SETEX", MasterKey, 60, myAddress); err != nil {
					conn.Close()
					return nil, errors.Wrap(err, "set master key with ttl")
				}
				go func() {
					defer conn.Close()
					for range time.Tick(45 * time.Second) {
						logrus.Debug("setting master key")
						if _, err := conn.Do("SETEX", MasterKey, 60, myAddress); err != nil {
							logrus.WithError(err).Error("set master key")
							conn.Close()
							conn = slave.Get()
						}
					}
				}()
			}
			return st, nil
		}
		return nil, errors.Wrap(err, "get master key")
	}
	st.m = redis.NewPool(func() (redis.Conn, error) {
		return redis.Dial("tcp", master)
	}, 5)
	return st, nil
}

type Store struct {
	m *redis.Pool
	s *redis.Pool
}

func (s *Store) Conn() redis.Conn {
	return s.s.Get()
}

func (s *Store) RWConn() redis.Conn {
	return s.m.Get()
}

func (s *Store) Close() error {
	s.s.Close()
	return s.m.Close()
}
