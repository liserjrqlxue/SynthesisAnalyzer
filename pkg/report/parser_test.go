package report

import (
	"testing"
)

func TestParseBatchInfo(t *testing.T) {
	// 测试用例：用户提供的文件路径
	testPath := "test/260305B-引物订购单_BOM.xlsx"
	batchID, synthesisDate, instrumentID, err := ParseBatchInfo(testPath)
	if err != nil {
		t.Errorf("解析失败: %v", err)
		return
	}

	// 验证结果
	expectedBatchID := "260305B"
	expectedSynthesisDate := "2026.03.05"
	expectedInstrumentID := "B"

	if batchID != expectedBatchID {
		t.Errorf("BatchID 错误: 期望 %s, 实际 %s", expectedBatchID, batchID)
	}

	if synthesisDate != expectedSynthesisDate {
		t.Errorf("合成日期 错误: 期望 %s, 实际 %s", expectedSynthesisDate, synthesisDate)
	}

	if instrumentID != expectedInstrumentID {
		t.Errorf("仪器号 错误: 期望 %s, 实际 %s", expectedInstrumentID, instrumentID)
	}

	// 打印测试结果
	t.Logf("测试通过! BatchID: %s, 合成日期: %s, 仪器号: %s", batchID, synthesisDate, instrumentID)
}

func TestParseBatchInfoWithDifferentFormats(t *testing.T) {
	// 测试其他可能的文件名格式
	testCases := []struct {
		path     string
		expectedBatchID     string
		expectedSynthesisDate string
		expectedInstrumentID  string
		expectedError        bool
	}{
		{
			path:                 "test/260305B-引物订购单_BOM.xlsx",
			expectedBatchID:     "260305B",
			expectedSynthesisDate: "2026.03.05",
			expectedInstrumentID:  "B",
			expectedError:        false,
		},
		{
			path:                 "test/260101A-测试_BOM.xlsx",
			expectedBatchID:     "260101A",
			expectedSynthesisDate: "2026.01.01",
			expectedInstrumentID:  "A",
			expectedError:        false,
		},
		{
			path:                 "test/261231Z-年末_BOM.xlsx",
			expectedBatchID:     "261231Z",
			expectedSynthesisDate: "2026.12.31",
			expectedInstrumentID:  "Z",
			expectedError:        false,
		},
		{
			path:                 "test/260305AB-双仪器_BOM.xlsx",
			expectedBatchID:     "260305AB",
			expectedSynthesisDate: "2026.03.05",
			expectedInstrumentID:  "AB",
			expectedError:        false,
		},
		{
			path:                 "test/invalid-文件名_BOM.xlsx",
			expectedError:        true,
		},
		{
			path:                 "test/260305-无仪器号_BOM.xlsx",
			expectedError:        true,
		},
	}

	for i, tc := range testCases {
		batchID, synthesisDate, instrumentID, err := ParseBatchInfo(tc.path)
		if tc.expectedError {
			if err == nil {
				t.Errorf("测试用例 %d: 期望错误，但未出现错误", i)
			}
		} else {
			if err != nil {
				t.Errorf("测试用例 %d: 未期望错误，但出现错误: %v", i, err)
				continue
			}
			if batchID != tc.expectedBatchID {
				t.Errorf("测试用例 %d: BatchID 错误: 期望 %s, 实际 %s", i, tc.expectedBatchID, batchID)
			}
			if synthesisDate != tc.expectedSynthesisDate {
				t.Errorf("测试用例 %d: 合成日期 错误: 期望 %s, 实际 %s", i, tc.expectedSynthesisDate, synthesisDate)
			}
			if instrumentID != tc.expectedInstrumentID {
				t.Errorf("测试用例 %d: 仪器号 错误: 期望 %s, 实际 %s", i, tc.expectedInstrumentID, instrumentID)
			}
		}
	}
}
