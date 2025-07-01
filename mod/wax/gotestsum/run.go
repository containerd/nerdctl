package gotestsum

type Run struct {
	Pipelines []*Pipeline
}

func (r *Run) NeverRan() []TestCase {
	var result []TestCase
	for _, pipe := range r.Pipelines {
		pipelineSkipped := []TestCase{}
		for _, pkg := range pipe.Packages {
			pipelineSkipped = append(pipelineSkipped, pkg.Skipped...)
		}

		if result == nil {
			result = append(result, pipelineSkipped...)
			continue
		}

		result = testTestIntersection(pipelineSkipped, result)
	}

	return result
}

func (r *Run) FailedAlways() []*FailedTest {
	var result []*FailedTest
	for _, pipe := range r.Pipelines {
		pipelineFailed := []*FailedTest{}
		for _, pkg := range pipe.Packages {
			for _, testCase := range pkg.Failed {
				ft := &FailedTest{
					TestCase: testCase,
					//nolint:staticcheck
					Output:       pkg.Output(testCase.ID),
					AlwaysFailed: true,
				}
				pipelineFailed = append(pipelineFailed, ft)
			}
		}
		if result == nil {
			result = append(result, pipelineFailed...)
			continue
		}

		result = testIntersection(pipelineFailed, result)
	}

	return result
}

func (r *Run) FailedAtLeastOnceButNotAlways() []*FailedTest {
	var result []*FailedTest
	for _, pipe := range r.Pipelines {
		pipelineFailed := []*FailedTest{}
		for _, pkg := range pipe.Packages {
			for _, testCase := range pkg.Failed {
				ft := &FailedTest{
					TestCase: testCase,
					//nolint:staticcheck
					Output: pkg.Output(testCase.ID),
				}
				pipelineFailed = append(pipelineFailed, ft)
			}
		}
		if result == nil {
			result = append(result, pipelineFailed...)
			continue
		}

		result = testUnion(pipelineFailed, result)
	}
	always := r.FailedAlways()
	result = testNotIn(always, result)

	return result
}

//	func (r *Run) Slowest(num int) map[string]float64 {
//		tests := map[string][]TestCase{}
//		times := map[string]float64{}
//		for _, pipe := range r.Pipelines {
//			for _, pkg := range pipe.Packages {
//				// Use only successful tests to calculate mean time
//				for _, test := range pkg.Passed {
//					fqn := test.Package + "/" + string(test.Test)
//					val, ok := tests[fqn]
//					if !ok {
//						tests[fqn] = []TestCase{}
//					}
//					tests[fqn] = append(val, test)
//				}
//			}
//		}
//
//		for fqn, val := range tests {
//			var tm time.Duration
//			for _, testCase := range val {
//				tm += testCase.Elapsed
//			}
//			times[fqn] = math.Round(float64(tm.Microseconds()) / float64(len(val)))
//		}
//
//		keys := make([]string, 0, len(times))
//		for k := range times {
//			keys = append(keys, k)
//		}
//
//		sort.Slice(keys, func(i, j int) bool {
//			return times[keys[i]] < times[keys[j]]
//		})
//
//		if num <= len(keys) {
//			keys = keys[:int(num)]
//		}
//		result := map[string]float64{}
//		for _, key := range keys {
//			result[key] = times[key]
//		}
//
//		return result
//	}

//func (r *Run) OutputForTest(testCase *TestCase) {
//	for _, pipe := range r.Pipelines {
//		for _, pkg := range pipe.Packages {
//			if testCase.Package == pkg {
//
//			}
//		}
//	}
//	//pkg.OutputLines(pkg.Failed[0])
//}
