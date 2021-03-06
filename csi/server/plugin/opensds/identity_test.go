// Copyright (c) 2018 Huawei Technologies Co., Ltd. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package opensds

import (
	"reflect"
	"testing"

	csi "github.com/container-storage-interface/spec/lib/go/csi/v0"
	"golang.org/x/net/context"
)

func TestGetPluginInfo(t *testing.T) {
	var fakePlugin = &Plugin{}
	var fakeCtx = context.Background()
	fakeReq := &csi.GetPluginInfoRequest{}

	expectedPluginInfo := &csi.GetPluginInfoResponse{
		Name:          PluginName,
		VendorVersion: "",
		Manifest:      nil,
	}

	rs, err := fakePlugin.GetPluginInfo(fakeCtx, fakeReq)
	if err != nil {
		t.Errorf("failed to GetPluginInfo: %v\n", err)
	}

	if !reflect.DeepEqual(rs, expectedPluginInfo) {
		t.Errorf("expected: %v, actual: %v\n", rs, expectedPluginInfo)
	}
}
