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

package mountutilmock

import (
	"os"
	"reflect"

	"go.uber.org/mock/gomock"
)

type MockOs struct {
	ctrl     *gomock.Controller
	recorder *MockOsMockRecorder
}

type MockOsMockRecorder struct {
	mock *MockOs
}

func NewMockOs(ctrl *gomock.Controller) *MockOs {
	mock := &MockOs{ctrl: ctrl}
	mock.recorder = &MockOsMockRecorder{mock}
	return mock
}

func (m *MockOs) EXPECT() *MockOsMockRecorder {
	return m.recorder
}

func (m *MockOs) Stat(name string) (os.FileInfo, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Stat", name)
	ret0, _ := ret[0].(os.FileInfo)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (m *MockOsMockRecorder) Stat(name any) *gomock.Call {
	m.mock.ctrl.T.Helper()
	return m.mock.ctrl.RecordCallWithMethodType(m.mock, "Stat", reflect.TypeOf((*MockOs)(nil).Stat), name)
}
