/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package serviceparser

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/compose-spec/compose-go/types"
	"github.com/containerd/nerdctl/pkg/composer/projectloader"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/containerd/nerdctl/pkg/testutil"
	"gotest.tools/v3/assert"
)

func TestServicePortConfigToFlagP(t *testing.T) {
	t.Parallel()
	type testCase struct {
		types.ServicePortConfig
		expected string
	}
	testCases := []testCase{
		{
			ServicePortConfig: types.ServicePortConfig{
				Mode:      "ingress",
				Target:    80,
				Published: 8080,
				Protocol:  "tcp",
			},
			expected: "8080:80/tcp",
		},
		{
			ServicePortConfig: types.ServicePortConfig{
				HostIP:    "127.0.0.1",
				Target:    80,
				Published: 8080,
			},
			expected: "127.0.0.1:8080:80",
		},
	}
	for i, tc := range testCases {
		got, err := servicePortConfigToFlagP(tc.ServicePortConfig)
		if tc.expected == "" {
			if err == nil {
				t.Errorf("#%d: error is expected", i)
			}
			continue
		}
		assert.NilError(t, err)
		assert.Equal(t, tc.expected, got)
	}
}

var in = strutil.InStringSlice

func TestParse(t *testing.T) {
	t.Parallel()
	const dockerComposeYAML = `
version: '3.1'

services:

  wordpress:
    image: wordpress:5.7
    restart: always
    ports:
      - 8080:80
    environment:
      WORDPRESS_DB_HOST: db
      WORDPRESS_DB_USER: exampleuser
      WORDPRESS_DB_PASSWORD: examplepass
      WORDPRESS_DB_NAME: exampledb
    volumes:
      - wordpress:/var/www/html
    pids_limit: 100

  db:
    image: mariadb:10.5
    restart: always
    environment:
      MYSQL_DATABASE: exampledb
      MYSQL_USER: exampleuser
      MYSQL_PASSWORD: examplepass
      MYSQL_RANDOM_ROOT_PASSWORD: '1'
    volumes:
      - db:/var/lib/mysql

volumes:
  wordpress:
  db:
`
	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	project, err := projectloader.Load(comp.YAMLFullPath(), comp.ProjectName(), nil)
	assert.NilError(t, err)

	wpSvc, err := project.GetService("wordpress")
	assert.NilError(t, err)

	wp, err := Parse(project, wpSvc)
	assert.NilError(t, err)

	t.Logf("wordpress: %+v", wp)
	assert.Assert(t, wp.PullMode == "missing")
	assert.Assert(t, wp.Image == "wordpress:5.7")
	assert.Assert(t, len(wp.Containers) == 1)
	wp1 := wp.Containers[0]
	assert.Assert(t, wp1.Name == fmt.Sprintf("%s_wordpress_1", project.Name))
	assert.Assert(t, in(wp1.RunArgs, "--name="+wp1.Name))
	assert.Assert(t, in(wp1.RunArgs, "--hostname=wordpress"))
	assert.Assert(t, in(wp1.RunArgs, fmt.Sprintf("--net=%s_default", project.Name)))
	assert.Assert(t, in(wp1.RunArgs, "--restart=always"))
	assert.Assert(t, in(wp1.RunArgs, "-e=WORDPRESS_DB_HOST=db"))
	assert.Assert(t, in(wp1.RunArgs, "-e=WORDPRESS_DB_USER=exampleuser"))
	assert.Assert(t, in(wp1.RunArgs, "-p=8080:80/tcp"))
	assert.Assert(t, in(wp1.RunArgs, fmt.Sprintf("-v=%s_wordpress:/var/www/html", project.Name)))
	assert.Assert(t, in(wp1.RunArgs, "--pids-limit=100"))

	dbSvc, err := project.GetService("db")
	assert.NilError(t, err)

	db, err := Parse(project, dbSvc)
	assert.NilError(t, err)

	t.Logf("db: %+v", db)
	assert.Assert(t, len(db.Containers) == 1)
	db1 := db.Containers[0]
	assert.Assert(t, db1.Name == fmt.Sprintf("%s_db_1", project.Name))
	assert.Assert(t, in(db1.RunArgs, "--hostname=db"))
	assert.Assert(t, in(db1.RunArgs, fmt.Sprintf("-v=%s_db:/var/lib/mysql", project.Name)))
}

func TestParseDeprecated(t *testing.T) {
	t.Parallel()
	const dockerComposeYAML = `
services:
  foo:
    image: nginx:alpine
    # scale is deprecated in favor of deploy.replicas, but still valid
    scale: 2
    # cpus is deprecated in favor of deploy.resources.limits.cpu, but still valid
    cpus: 0.42
    # mem_limit is deprecated in favor of deploy.resources.limits.memory, but still valid
    mem_limit: 42m
`
	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	project, err := projectloader.Load(comp.YAMLFullPath(), comp.ProjectName(), nil)
	assert.NilError(t, err)

	fooSvc, err := project.GetService("foo")
	assert.NilError(t, err)

	foo, err := Parse(project, fooSvc)
	assert.NilError(t, err)

	t.Logf("foo: %+v", foo)
	assert.Assert(t, len(foo.Containers) == 2)
	for i, c := range foo.Containers {
		assert.Assert(t, c.Name == fmt.Sprintf("%s_foo_%d", project.Name, i+1))
		assert.Assert(t, in(c.RunArgs, "--name="+c.Name))
		assert.Assert(t, in(c.RunArgs, fmt.Sprintf("--cpus=%f", 0.42)))
		assert.Assert(t, in(c.RunArgs, "-m=44040192"))
	}
}

func TestParseDeploy(t *testing.T) {
	t.Parallel()
	const dockerComposeYAML = `
services:
  foo: # restart=no
    image: nginx:alpine
    deploy:
      replicas: 3
      resources:
        limits:
          cpus: "0.42"
          memory: "42m"
  bar: # restart=always
    image: nginx:alpine
    deploy:
      restart_policy: {}
      resources:
        reservations:
          devices:
          - capabilities: ["gpu", "utility", "compute"]
            driver: nvidia
            count: 2
          - capabilities: ["nvidia"]
            device_ids: ["dummy", "dummy2"]
  baz: # restart=no
    image: nginx:alpine
    deploy:
      restart_policy:
        condition: none
      resources:
        reservations:
          devices:
          - capabilities: ["utility"]
            count: all
`
	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	project, err := projectloader.Load(comp.YAMLFullPath(), comp.ProjectName(), nil)
	assert.NilError(t, err)

	fooSvc, err := project.GetService("foo")
	assert.NilError(t, err)

	foo, err := Parse(project, fooSvc)
	assert.NilError(t, err)

	t.Logf("foo: %+v", foo)
	assert.Assert(t, len(foo.Containers) == 3)
	for i, c := range foo.Containers {
		assert.Assert(t, c.Name == fmt.Sprintf("%s_foo_%d", project.Name, i+1))
		assert.Assert(t, in(c.RunArgs, "--name="+c.Name))

		assert.Assert(t, in(c.RunArgs, "--restart=no"))
		assert.Assert(t, in(c.RunArgs, "--cpus=0.42"))
		assert.Assert(t, in(c.RunArgs, "-m=44040192"))
	}

	barSvc, err := project.GetService("bar")
	assert.NilError(t, err)

	bar, err := Parse(project, barSvc)
	assert.NilError(t, err)

	t.Logf("bar: %+v", bar)
	assert.Assert(t, len(bar.Containers) == 1)
	for _, c := range bar.Containers {
		assert.Assert(t, in(c.RunArgs, "--restart=always"))
		assert.Assert(t, in(c.RunArgs, `--gpus="capabilities=gpu,utility,compute",driver=nvidia,count=2`))
		assert.Assert(t, in(c.RunArgs, `--gpus=capabilities=nvidia,"device=dummy,dummy2"`))
	}

	bazSvc, err := project.GetService("baz")
	assert.NilError(t, err)

	baz, err := Parse(project, bazSvc)
	assert.NilError(t, err)

	t.Logf("baz: %+v", baz)
	assert.Assert(t, len(baz.Containers) == 1)
	for _, c := range baz.Containers {
		assert.Assert(t, in(c.RunArgs, "--restart=no"))
		assert.Assert(t, in(c.RunArgs, `--gpus=capabilities=utility,count=-1`))
	}
}

func TestParseRelative(t *testing.T) {
	t.Parallel()
	const dockerComposeYAML = `
services:
  foo:
    image: nginx:alpine
    volumes:
    - "/file1:/file1"
    - "./file2:/file2"
    # break out the project dir, but this is fine
    - "../../../../../../../../../../../../../../../../../../../../../../../../../../../../../../../../../../../file3:/file3"
`
	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	project, err := projectloader.Load(comp.YAMLFullPath(), comp.ProjectName(), nil)
	assert.NilError(t, err)

	fooSvc, err := project.GetService("foo")
	assert.NilError(t, err)

	foo, err := Parse(project, fooSvc)
	assert.NilError(t, err)

	t.Logf("foo: %+v", foo)
	for _, c := range foo.Containers {
		assert.Assert(t, in(c.RunArgs, "-v=/file1:/file1"))
		assert.Assert(t, in(c.RunArgs, fmt.Sprintf("-v=%s:/file2", filepath.Join(project.WorkingDir, "file2"))))
		assert.Assert(t, in(c.RunArgs, "-v=/file3:/file3"))
	}
}

func TestParseNetworkMode(t *testing.T) {
	t.Parallel()
	const dockerComposeYAML = `
services:
  foo:
    image: nginx:alpine
    network_mode: host
`
	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	project, err := projectloader.Load(comp.YAMLFullPath(), comp.ProjectName(), nil)
	assert.NilError(t, err)

	fooSvc, err := project.GetService("foo")
	assert.NilError(t, err)

	foo, err := Parse(project, fooSvc)
	assert.NilError(t, err)

	t.Logf("foo: %+v", foo)
	for _, c := range foo.Containers {
		assert.Assert(t, in(c.RunArgs, "--net=host"))
	}
}

func TestParseConfigs(t *testing.T) {
	t.Parallel()
	const dockerComposeYAML = `
services:
  foo:
    image: nginx:alpine
    secrets:
    - secret1
    - source: secret2
      target: secret2-foo
    - source: secret3
      target: /mnt/secret3-foo
    configs:
    - config1
    - source: config2
      target: /mnt/config2-foo
secrets:
  secret1:
    file: ./secret1
  secret2:
    file: ./secret2
  secret3:
    file: ./secret3
configs:
  config1:
    file: ./config1
  config2:
    file: ./config2
`
	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	project, err := projectloader.Load(comp.YAMLFullPath(), comp.ProjectName(), nil)
	assert.NilError(t, err)

	for _, f := range []string{"secret1", "secret2", "secret3", "config1", "config2"} {
		err = ioutil.WriteFile(filepath.Join(project.WorkingDir, f), []byte("content-"+f), 0444)
		assert.NilError(t, err)
	}

	fooSvc, err := project.GetService("foo")
	assert.NilError(t, err)

	foo, err := Parse(project, fooSvc)
	assert.NilError(t, err)

	t.Logf("foo: %+v", foo)
	for _, c := range foo.Containers {
		assert.Assert(t, in(c.RunArgs, fmt.Sprintf("-v=%s:/run/secrets/secret1:ro", filepath.Join(project.WorkingDir, "secret1"))))
		assert.Assert(t, in(c.RunArgs, fmt.Sprintf("-v=%s:/run/secrets/secret2-foo:ro", filepath.Join(project.WorkingDir, "secret2"))))
		assert.Assert(t, in(c.RunArgs, fmt.Sprintf("-v=%s:/mnt/secret3-foo:ro", filepath.Join(project.WorkingDir, "secret3"))))
		assert.Assert(t, in(c.RunArgs, fmt.Sprintf("-v=%s:/config1:ro", filepath.Join(project.WorkingDir, "config1"))))
		assert.Assert(t, in(c.RunArgs, fmt.Sprintf("-v=%s:/mnt/config2-foo:ro", filepath.Join(project.WorkingDir, "config2"))))
	}
}
