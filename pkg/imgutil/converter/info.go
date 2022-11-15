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

package converter

// ConvertedImageInfo is information of the images created by a conversion.
type ConvertedImageInfo struct {
	// Image is the reference of the converted image.
	// The reference is the image's name and digest concatenated with "@" (i.e. `<name>@<digest>`).
	Image string `json:"Image"`

	// ExtraImages is a set of converter-specific additional images (e.g. external TOC image of eStargz).
	// The reference format is the same as the "Image" field.
	ExtraImages []string `json:"ExtraImages"`
}
