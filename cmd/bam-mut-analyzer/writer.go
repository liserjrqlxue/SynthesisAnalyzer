package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// writePositionStats 写入各bam各位置各碱基变化组合的个数分布统计
func writePositionStats(stats *MutationStats, outputDir string) error {
	// 确定样本顺序
	var sampleNames []string
	if excelFile != "" && len(sampleOrder) > 0 {
		// 使用Excel中的样本顺序
		sampleNames = sampleOrder
	} else {
		// 收集所有样本名并按字母排序
		for sampleName := range stats.PositionDetails {
			sampleNames = append(sampleNames, sampleName)
		}
		sort.Strings(sampleNames)
	}

	for _, sampleName := range sampleNames {
		posDetails, exists := stats.PositionDetails[sampleName]
		if !exists {
			continue
		}

		filename := filepath.Join(outputDir, fmt.Sprintf("%s_position_stats.csv", sampleName))
		filename2 := filepath.Join(outputDir, fmt.Sprintf("%s_position_stats2.csv", sampleName))
		file, err := os.Create(filename)
		if err != nil {
			return fmt.Errorf("创建文件失败 %s: %v", filename, err)
		}
		defer file.Close()
		file2, err := os.Create(filename2)
		if err != nil {
			return fmt.Errorf("创建文件失败 %s: %v", filename2, err)
		}
		defer file2.Close()

		writer := bufio.NewWriter(file)
		writer.WriteString("Sample,Position,Ref>Alt,Count,TotalReads,ReadsWithMutations\n")
		writer2 := bufio.NewWriter(file2)
		writer2.WriteString("Sample,Position,Count,TotalReads,ReadsWithMutations\n")

		// 获取样本的总reads数和包含突变的reads数
		totalReads := stats.SampleReadCounts[sampleName]
		readsWithMuts := stats.SampleReadsWithMuts[sampleName]

		// 收集所有位置
		var positions []string
		for pos := range posDetails {
			positions = append(positions, pos)
		}

		// 按位置排序
		sort.Slice(positions, func(i, j int) bool {
			numI, _ := strconv.Atoi(positions[i])
			numJ, _ := strconv.Atoi(positions[j])
			return numI < numJ
		})

		for _, pos := range positions {
			// 收集该位置的所有突变
			var mutations []string
			for mut := range posDetails[pos] {
				mutations = append(mutations, mut)
			}
			sort.Strings(mutations)

			count2 := 0
			for _, mut := range mutations {
				count := posDetails[pos][mut]
				fmt.Fprintf(writer, "%s,%s,%s,%d,%d,%d\n",
					sampleName, pos, mut, count, totalReads, readsWithMuts)
				count2 += count
			}
			fmt.Fprintf(writer2, "%s,%s,%d,%d,%d\n",
				sampleName, pos, count2, totalReads, readsWithMuts)
		}

		writer.Flush()
		fmt.Printf("  已写入: %s\n", filename)
		writer2.Flush()
		fmt.Printf("  已写入: %s\n", filename2)
	}

	return nil
}

// writeSampleMutationStats 写入各bam各碱基变化组合的分布
func writeSampleMutationStats(stats *MutationStats, outputDir string) error {
	// 确定样本顺序
	var sampleNames []string
	if excelFile != "" && len(sampleOrder) > 0 {
		// 使用Excel中的样本顺序
		sampleNames = sampleOrder
	} else {
		// 收集所有样本名并按字母排序
		for sampleName := range stats.SampleMutations {
			sampleNames = append(sampleNames, sampleName)
		}
		sort.Strings(sampleNames)
	}

	for _, sampleName := range sampleNames {
		mutCounts, exists := stats.SampleMutations[sampleName]
		if !exists {
			continue
		}

		filename := filepath.Join(outputDir, fmt.Sprintf("%s_mutation_stats.csv", sampleName))
		file, err := os.Create(filename)
		if err != nil {
			return fmt.Errorf("创建文件失败 %s: %v", filename, err)
		}
		defer file.Close()

		writer := bufio.NewWriter(file)
		writer.WriteString("Sample,Ref>Alt,Count,TotalReads,ReadsWithMutations\n")

		// 获取样本的总reads数和包含突变的reads数
		totalReads := stats.SampleReadCounts[sampleName]
		readsWithMuts := stats.SampleReadsWithMuts[sampleName]

		// 收集所有突变
		var mutations []string
		for mut := range mutCounts {
			mutations = append(mutations, mut)
		}
		sort.Strings(mutations)

		for _, mut := range mutations {
			count := mutCounts[mut]
			fmt.Fprintf(writer, "%s,%s,%d,%d,%d\n",
				sampleName, mut, count, totalReads, readsWithMuts)
		}

		writer.Flush()
		fmt.Printf("  已写入: %s\n", filename)
	}

	return nil
}

// writeTotalMutationStats 写入总碱基变化组合的分布
func writeTotalMutationStats(stats *MutationStats, outputDir string) error {
	filename := filepath.Join(outputDir, "total_mutation_stats.csv")
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	writer.WriteString("Ref>Alt,Count,TotalReads,TotalReadsWithMutations\n")

	// 收集所有突变
	var mutations []string
	for mut := range stats.TotalMutations {
		mutations = append(mutations, mut)
	}
	sort.Strings(mutations)

	// 获取所有样本的总reads数和包含突变reads数的和
	totalReads := stats.TotalReadCount
	totalReadsWithMuts := stats.TotalReadsWithMuts

	for _, mut := range mutations {
		count := stats.TotalMutations[mut]
		fmt.Fprintf(writer, "%s,%d,%d,%d\n",
			mut, count, totalReads, totalReadsWithMuts)
	}

	writer.Flush()
	fmt.Printf("总统计已写入: %s\n", filename)

	return nil
}

// writeSummaryReport 写入汇总报告 - 更新版本
func writeSummaryReport(stats *MutationStats, outputDir string) error {
	// 1. 样本汇总（突变类型统计）
	filename := filepath.Join(outputDir, "summary_by_sample.csv")
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	// 收集所有突变类型（包括通过MD发现的）
	allMutations := make(map[string]bool)
	for _, sampleMuts := range stats.SampleMutations {
		for mut := range sampleMuts {
			allMutations[mut] = true
		}
	}

	// 转换为排序的切片
	var mutationTypes []string
	for mut := range allMutations {
		mutationTypes = append(mutationTypes, mut)
	}
	sort.Strings(mutationTypes)

	// 写入表头
	header := "Sample,Total_Reads,Reads_With_Mutations,Total_Mutations"
	if len(mutationTypes) > 0 {
		header += "," + strings.Join(mutationTypes, ",")
	}
	writer.WriteString(header + "\n")

	// 确定样本顺序
	var sampleNames []string
	if excelFile != "" && len(sampleOrder) > 0 {
		// 使用Excel中的样本顺序
		sampleNames = sampleOrder
	} else {
		// 收集所有样本名并按字母排序
		for sampleName := range stats.SampleMutations {
			sampleNames = append(sampleNames, sampleName)
		}
		sort.Strings(sampleNames)
	}

	// 写入每行数据
	for _, sampleName := range sampleNames {
		sampleMuts, exists := stats.SampleMutations[sampleName]
		if !exists {
			continue
		}

		totalReads := stats.SampleReadCounts[sampleName]
		readsWithMuts := stats.SampleReadsWithMuts[sampleName]
		totalMutations := 0
		for _, count := range sampleMuts {
			totalMutations += count
		}

		var row strings.Builder
		fmt.Fprintf(&row, "%s,%d,%d,%d", sampleName, totalReads, readsWithMuts, totalMutations)
		for _, mut := range mutationTypes {
			count := sampleMuts[mut]
			fmt.Fprintf(&row, ",%d", count)
		}
		writer.WriteString(row.String() + "\n")
	}

	writer.Flush()
	fmt.Printf("样本汇总已写入: %s\n", filename)

	// 2. 突变频谱（按频率排序）
	filename = filepath.Join(outputDir, "mutation_spectrum.csv")
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer = bufio.NewWriter(file)
	writer.WriteString("Mutation,Count,Frequency\n")

	// 创建突变类型和计数的切片用于排序
	type mutationCount struct {
		mutation string
		count    int
	}

	var mutationCounts []mutationCount
	for mut, count := range stats.TotalMutations {
		mutationCounts = append(mutationCounts, mutationCount{mut, count})
	}

	// 按计数降序排序
	sort.Slice(mutationCounts, func(i, j int) bool {
		return mutationCounts[i].count > mutationCounts[j].count
	})

	totalMutations := 0
	for _, count := range stats.TotalMutations {
		totalMutations += count
	}

	// 写入排序后的数据
	for _, mc := range mutationCounts {
		count := mc.count
		frequency := float64(count) / float64(totalMutations)
		percentage := frequency * 100
		fmt.Fprintf(writer, "%s,%d,%.6f,%.4f%%\n",
			mc.mutation, count, frequency, percentage)
	}
	writer.Flush()
	fmt.Printf("突变频谱已写入: %s\n", filename)

	// 3. 突变类型分析（transition/transversion等）
	filename = filepath.Join(outputDir, "mutation_analysis.csv")
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer = bufio.NewWriter(file)
	writer.WriteString("MutationType,Count,Percentage\n")

	// 分析突变类型
	var transitions, transversions, other int
	for mut, count := range stats.TotalMutations {
		if len(mut) >= 3 && mut[1] == '>' {
			ref := mut[0]
			alt := mut[2]

			// 转换（嘌呤<->嘌呤或嘧啶<->嘧啶）
			if (ref == 'A' && alt == 'G') || (ref == 'G' && alt == 'A') ||
				(ref == 'C' && alt == 'T') || (ref == 'T' && alt == 'C') {
				transitions += count
			} else if (ref == 'A' && alt == 'C') || (ref == 'A' && alt == 'T') ||
				(ref == 'G' && alt == 'C') || (ref == 'G' && alt == 'T') ||
				(ref == 'C' && alt == 'A') || (ref == 'C' && alt == 'G') ||
				(ref == 'T' && alt == 'A') || (ref == 'T' && alt == 'G') {
				transversions += count
			} else {
				other += count
			}
		} else {
			other += count
		}
	}

	total := transitions + transversions + other
	if total > 0 {
		fmt.Fprintf(writer, "Transitions,%d,%.4f%%\n",
			transitions, float64(transitions)/float64(total)*100)
		fmt.Fprintf(writer, "Transversions,%d,%.4f%%\n",
			transversions, float64(transversions)/float64(total)*100)
		fmt.Fprintf(writer, "Other,%d,%.4f%%\n",
			other, float64(other)/float64(total)*100)
		fmt.Fprintf(writer, "Ti/Tv Ratio,%.4f,\n",
			float64(transitions)/float64(transversions))
	}

	writer.Flush()
	fmt.Printf("突变分析已写入: %s\n", filename)

	return nil
}

// writeReadTypeStats 写入read类型统计
func writeReadTypeStats(stats *MutationStats, outputDir string) error {
	// 1. 各样本的read类型统计
	filename := filepath.Join(outputDir, "read_type_by_sample.csv")
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	// 写入表头
	var header strings.Builder
	header.WriteString("Sample,TotalReads")
	for rt := ReadTypeMatch; rt <= ReadTypeAll; rt++ {
		header.WriteString("," + ReadTypeNames[rt])
	}
	header.WriteString(",ReadsWithMutations")
	writer.WriteString(header.String() + "\n")

	// 确定样本顺序
	var sampleNames []string
	if excelFile != "" && len(sampleOrder) > 0 {
		// 使用Excel中的样本顺序
		sampleNames = sampleOrder
	} else {
		// 收集所有样本名并按字母排序
		for sampleName := range stats.SampleReadTypeCounts {
			sampleNames = append(sampleNames, sampleName)
		}
		sort.Strings(sampleNames)
	}

	// 写入每行数据
	for _, sampleName := range sampleNames {
		readTypeCounts, exists := stats.SampleReadTypeCounts[sampleName]
		if !exists {
			continue
		}

		totalReads := stats.SampleReadCounts[sampleName]
		readsWithMuts := stats.SampleReadsWithMuts[sampleName]

		row := fmt.Sprintf("%s,%d", sampleName, totalReads)

		// 写入每种read类型的计数
		for rt := ReadTypeMatch; rt <= ReadTypeAll; rt++ {
			count := readTypeCounts[rt]
			row += fmt.Sprintf(",%d", count)
		}

		row += fmt.Sprintf(",%d", readsWithMuts)
		writer.WriteString(row + "\n")
	}

	writer.Flush()
	fmt.Printf("样本read类型统计已写入: %s\n", filename)

	// 2. 汇总的read类型统计
	filename = filepath.Join(outputDir, "read_type_summary.csv")
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer = bufio.NewWriter(file)

	// 写入表头
	writer.WriteString("ReadType,Count,Percentage,PercentageInTotalReads\n")

	totalReads := stats.TotalReadCount

	for rt := ReadTypeMatch; rt <= ReadTypeAll; rt++ {
		count := stats.TotalReadTypeCounts[rt]
		percentage := float64(count) / float64(totalReads) * 100

		// 计算在总reads中的百分比
		percentageInTotal := percentage

		writer.WriteString(fmt.Sprintf("%s,%d,%.4f%%,%.4f%%\n",
			ReadTypeNames[rt], count, percentage, percentageInTotal))
	}

	// 添加包含突变的reads统计
	readsWithMuts := stats.TotalReadsWithMuts
	percentageWithMuts := float64(readsWithMuts) / float64(totalReads) * 100
	writer.WriteString(fmt.Sprintf("包含突变的reads,%d,%.4f%%,%.4f%%\n",
		readsWithMuts, percentageWithMuts, percentageWithMuts))

	writer.Flush()
	fmt.Printf("汇总read类型统计已写入: %s\n", filename)

	// 3. 详细的read类型统计（包含频率和百分比）
	filename = filepath.Join(outputDir, "read_type_detailed.csv")
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer = bufio.NewWriter(file)

	// 写入表头
	writer.WriteString("ReadType,Sample")
	for _, sampleName := range sampleNames {
		fmt.Fprintf(writer, ",%s_Count,%s_Percentage", sampleName, sampleName)
	}
	writer.WriteString(",Total_Count,Total_Percentage\n")

	// 写入每种read类型的数据
	for rt := ReadTypeMatch; rt <= ReadTypeAll; rt++ {
		var row strings.Builder
		fmt.Fprintf(&row, "%s,", ReadTypeNames[rt])

		// 写入每个样本的计数和百分比
		for _, sampleName := range sampleNames {
			sampleReadTypeCounts, exists := stats.SampleReadTypeCounts[sampleName]
			if !exists {
				row.WriteString("0,0.00%,")
				continue
			}

			count := sampleReadTypeCounts[rt]
			sampleTotalReads := stats.SampleReadCounts[sampleName]
			percentage := 0.0
			if sampleTotalReads > 0 {
				percentage = float64(count) / float64(sampleTotalReads) * 100
			}

			fmt.Fprintf(&row, "%d,%.2f%%,", count, percentage)
		}

		// 写入总计
		totalCount := stats.TotalReadTypeCounts[rt]
		totalPercentage := float64(totalCount) / float64(totalReads) * 100
		fmt.Fprintf(&row, "%d,%.2f%%", totalCount, totalPercentage)

		writer.WriteString(row.String() + "\n")
	}

	// 添加包含突变的reads行
	var row strings.Builder
	row.WriteString("包含突变的reads,")
	for _, sampleName := range sampleNames {
		readsWithMuts := stats.SampleReadsWithMuts[sampleName]
		sampleTotalReads := stats.SampleReadCounts[sampleName]
		percentage := 0.0
		if sampleTotalReads > 0 {
			percentage = float64(readsWithMuts) / float64(sampleTotalReads) * 100
		}

		fmt.Fprintf(&row, "%d,%.2f%%,", readsWithMuts, percentage)
	}

	fmt.Fprintf(&row, "%d,%.2f%%", stats.TotalReadsWithMuts,
		float64(stats.TotalReadsWithMuts)/float64(totalReads)*100)

	writer.WriteString(row.String() + "\n")

	writer.Flush()
	fmt.Printf("详细read类型统计已写入: %s\n", filename)

	return nil
}

// writeDetailedStats 写入详细统计信息
func writeDetailedStats(stats *MutationStats, outputDir string) error {
	// 1. 插入长度分布
	filename := filepath.Join(outputDir, "insert_length_distribution.csv")
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	writer.WriteString("InsertLength,SampleCount,TotalCount,SamplePercentage,TotalPercentage\n")

	// 插入长度标签
	lengthLabels := map[int]string{
		1: "1",
		2: "2",
		3: "3",
		4: ">3",
	}

	for _, length := range []int{1, 2, 3, 4} {
		totalCount := stats.TotalInsertLengthDist[length]
		totalPercentage := 0.0
		if stats.TotalAlignedReads > 0 {
			totalPercentage = float64(totalCount) / float64(stats.TotalAlignedReads) * 100
		}

		writer.WriteString(fmt.Sprintf("%s,", lengthLabels[length]))

		// 各样本计数
		sampleCounts := []string{}
		for sampleName := range stats.SampleInsertLengthDist {
			count := stats.SampleInsertLengthDist[sampleName][length]
			sampleCounts = append(sampleCounts, fmt.Sprintf("%s:%d", sampleName, count))
		}
		writer.WriteString(strings.Join(sampleCounts, ";") + ",")

		writer.WriteString(fmt.Sprintf("%d,", totalCount))

		// 各样本百分比（相对于比对reads）
		samplePercents := []string{}
		for sampleName := range stats.SampleInsertLengthDist {
			count := stats.SampleInsertLengthDist[sampleName][length]
			percent := 0.0
			if stats.SampleAlignedReads[sampleName] > 0 {
				percent = float64(count) / float64(stats.SampleAlignedReads[sampleName]) * 100
			}
			samplePercents = append(samplePercents, fmt.Sprintf("%s:%.2f%%", sampleName, percent))
		}
		writer.WriteString(strings.Join(samplePercents, ";") + ",")

		writer.WriteString(fmt.Sprintf("%.4f%%\n", totalPercentage))
	}

	writer.Flush()
	fmt.Printf("插入长度分布已写入: %s\n", filename)

	// 2. 缺失长度分布
	filename = filepath.Join(outputDir, "delete_length_distribution.csv")
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer = bufio.NewWriter(file)
	writer.WriteString("DeleteLength,SampleCount,TotalCount,SamplePercentage,TotalPercentage\n")

	for _, length := range []int{1, 2, 3, 4} {
		totalCount := stats.TotalDeleteLengthDist[length]
		totalPercentage := 0.0
		if stats.TotalAlignedReads > 0 {
			totalPercentage = float64(totalCount) / float64(stats.TotalAlignedReads) * 100
		}

		writer.WriteString(fmt.Sprintf("%s,", lengthLabels[length]))

		// 各样本计数
		sampleCounts := []string{}
		for sampleName := range stats.SampleDeleteLengthDist {
			count := stats.SampleDeleteLengthDist[sampleName][length]
			sampleCounts = append(sampleCounts, fmt.Sprintf("%s:%d", sampleName, count))
		}
		writer.WriteString(strings.Join(sampleCounts, ";") + ",")

		writer.WriteString(fmt.Sprintf("%d,", totalCount))

		// 各样本百分比
		samplePercents := []string{}
		for sampleName := range stats.SampleDeleteLengthDist {
			count := stats.SampleDeleteLengthDist[sampleName][length]
			percent := 0.0
			if stats.SampleAlignedReads[sampleName] > 0 {
				percent = float64(count) / float64(stats.SampleAlignedReads[sampleName]) * 100
			}
			samplePercents = append(samplePercents, fmt.Sprintf("%s:%.2f%%", sampleName, percent))
		}
		writer.WriteString(strings.Join(samplePercents, ";") + ",")

		writer.WriteString(fmt.Sprintf("%.4f%%\n", totalPercentage))
	}

	writer.Flush()
	fmt.Printf("缺失长度分布已写入: %s\n", filename)

	// 3. 最长缺失长度分布
	filename = filepath.Join(outputDir, "max_delete_length_distribution.csv")
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer = bufio.NewWriter(file)
	writer.WriteString("MaxDeleteLength,SampleCount,TotalCount,SamplePercentage,TotalPercentage\n")

	for _, length := range []int{1, 2, 3, 4} {
		totalCount := stats.TotalMaxDeleteLengthDist[length]
		totalPercentage := 0.0
		if stats.TotalAlignedReads > 0 {
			totalPercentage = float64(totalCount) / float64(stats.TotalAlignedReads) * 100
		}
		writer.WriteString(fmt.Sprintf("%s,", lengthLabels[length]))
		writer.WriteString(fmt.Sprintf("%d,", totalCount))
		writer.WriteString(fmt.Sprintf("%.4f%%\n", totalPercentage))
	}

	writer.Flush()
	fmt.Printf("最长缺失长度分布已写入: %s\n", filename)

	// 4. 详细类型组合统计
	filename = filepath.Join(outputDir, "detailed_type_combinations.csv")
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer = bufio.NewWriter(file)
	writer.WriteString("DetailedType,SampleCount,TotalCount,SamplePercentage,TotalPercentage\n")

	// 收集所有详细类型
	allDetailedTypes := make(map[string]bool)
	for sampleName := range stats.SampleDetailedTypeCounts {
		for typeKey := range stats.SampleDetailedTypeCounts[sampleName] {
			allDetailedTypes[typeKey] = true
		}
	}

	// 转换为排序的切片
	var detailedTypes []string
	for typeKey := range allDetailedTypes {
		detailedTypes = append(detailedTypes, typeKey)
	}
	sort.Strings(detailedTypes)

	for _, typeKey := range detailedTypes {
		totalCount := stats.TotalDetailedTypeCounts[typeKey]
		totalPercentage := 0.0
		if stats.TotalAlignedReads > 0 {
			totalPercentage = float64(totalCount) / float64(stats.TotalAlignedReads) * 100
		}

		writer.WriteString(fmt.Sprintf("%s,", typeKey))

		// 各样本计数
		sampleCounts := []string{}
		for sampleName := range stats.SampleDetailedTypeCounts {
			count := stats.SampleDetailedTypeCounts[sampleName][typeKey]
			if count > 0 {
				sampleCounts = append(sampleCounts, fmt.Sprintf("%s:%d", sampleName, count))
			}
		}
		writer.WriteString(strings.Join(sampleCounts, ";") + ",")

		writer.WriteString(fmt.Sprintf("%d,", totalCount))

		// 各样本百分比
		samplePercents := []string{}
		for sampleName := range stats.SampleDetailedTypeCounts {
			count := stats.SampleDetailedTypeCounts[sampleName][typeKey]
			if count > 0 {
				percent := 0.0
				if stats.SampleAlignedReads[sampleName] > 0 {
					percent = float64(count) / float64(stats.SampleAlignedReads[sampleName]) * 100
				}
				samplePercents = append(samplePercents, fmt.Sprintf("%s:%.2f%%", sampleName, percent))
			}
		}
		writer.WriteString(strings.Join(samplePercents, ";") + ",")

		writer.WriteString(fmt.Sprintf("%.4f%%\n", totalPercentage))
	}

	writer.Flush()
	fmt.Printf("详细类型组合统计已写入: %s\n", filename)

	// 5. 比对率和变异比例（分母为比对reads）
	filename = filepath.Join(outputDir, "alignment_and_mutation_rates.csv")
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer = bufio.NewWriter(file)
	writer.WriteString("Sample,TotalReads,AlignedReads,AlignmentRate,ReadsWithMutations,MutationReadRate,TotalMutations,MutationRatePerRead\n")

	// 确定样本顺序
	var sampleNames []string
	if excelFile != "" && len(sampleOrder) > 0 {
		sampleNames = sampleOrder
	} else {
		for sampleName := range stats.SampleReadCounts {
			sampleNames = append(sampleNames, sampleName)
		}
		sort.Strings(sampleNames)
	}

	for _, sampleName := range sampleNames {
		totalReads := stats.SampleReadCounts[sampleName]
		alignedReads := stats.SampleAlignedReads[sampleName]
		readsWithMuts := stats.SampleReadsWithMuts[sampleName]

		// 计算总突变数
		totalMutations := 0
		if sampleMuts, exists := stats.SampleMutations[sampleName]; exists {
			for _, count := range sampleMuts {
				totalMutations += count
			}
		}

		alignmentRate := 0.0
		mutationReadRate := 0.0
		mutationRatePerRead := 0.0

		if totalReads > 0 {
			alignmentRate = float64(alignedReads) / float64(totalReads) * 100
		}
		if alignedReads > 0 {
			mutationReadRate = float64(readsWithMuts) / float64(alignedReads) * 100
			mutationRatePerRead = float64(totalMutations) / float64(alignedReads)
		}

		writer.WriteString(fmt.Sprintf("%s,%d,%d,%.4f%%,%d,%.4f%%,%d,%.4f\n",
			sampleName, totalReads, alignedReads, alignmentRate,
			readsWithMuts, mutationReadRate, totalMutations, mutationRatePerRead))
	}

	// 总计行
	totalAlignmentRate := 0.0
	totalMutationReadRate := 0.0
	totalMutationRatePerRead := 0.0

	if stats.TotalReadCount > 0 {
		totalAlignmentRate = float64(stats.TotalAlignedReads) / float64(stats.TotalReadCount) * 100
	}
	totalMutations := 0
	for _, count := range stats.TotalMutations {
		totalMutations += count
	}
	if stats.TotalAlignedReads > 0 {
		totalMutationReadRate = float64(stats.TotalReadsWithMuts) / float64(stats.TotalAlignedReads) * 100

		totalMutationRatePerRead = float64(totalMutations) / float64(stats.TotalAlignedReads)
	}

	writer.WriteString(fmt.Sprintf("Total,%d,%d,%.4f%%,%d,%.4f%%,%d,%.4f\n",
		stats.TotalReadCount, stats.TotalAlignedReads, totalAlignmentRate,
		stats.TotalReadsWithMuts, totalMutationReadRate, totalMutations, totalMutationRatePerRead))

	writer.Flush()
	fmt.Printf("比对率和变异比例已写入: %s\n", filename)

	return nil
}

// writeBaseMutationStats 写入碱基维度的变异统计
func writeBaseMutationStats(stats *MutationStats, outputDir string) error {
	// 1. 碱基维度的变异类型统计
	filename := filepath.Join(outputDir, "base_mutation_counts.csv")
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	writer.WriteString("Sample,TotalReads,AlignedReads,SubstitutionBases,InsertionBases,DeletionBases,SubstitutionRate,InsertionRate,DeletionRate\n")

	// 变异类型
	// mutationTypes := []string{"substitution", "insertion", "deletion"}

	// 确定样本顺序
	var sampleNames []string
	if excelFile != "" && len(sampleOrder) > 0 {
		sampleNames = sampleOrder
	} else {
		for sampleName := range stats.SampleReadCounts {
			sampleNames = append(sampleNames, sampleName)
		}
		sort.Strings(sampleNames)
	}

	for _, sampleName := range sampleNames {
		totalReads := stats.SampleReadCounts[sampleName]
		alignedReads := stats.SampleAlignedReads[sampleName]

		// 获取碱基计数
		substitutionBases := stats.MutationBaseCounts[sampleName]["substitution"]
		insertionBases := stats.MutationBaseCounts[sampleName]["insertion"]
		deletionBases := stats.MutationBaseCounts[sampleName]["deletion"]

		// 计算比例（每千碱基）
		substitutionRate := 0.0
		insertionRate := 0.0
		deletionRate := 0.0

		if alignedReads > 0 {
			// 假设平均read长度，这里简化处理
			// 实际应该计算总比对碱基数
			substitutionRate = float64(substitutionBases) / float64(alignedReads) * 1000
			insertionRate = float64(insertionBases) / float64(alignedReads) * 1000
			deletionRate = float64(deletionBases) / float64(alignedReads) * 1000
		}

		writer.WriteString(fmt.Sprintf("%s,%d,%d,%d,%d,%d,%.4f,%.4f,%.4f\n",
			sampleName, totalReads, alignedReads,
			substitutionBases, insertionBases, deletionBases,
			substitutionRate, insertionRate, deletionRate))
	}

	// 总计行
	totalSubstitution := 0
	totalInsertion := 0
	totalDeletion := 0

	for _, counts := range stats.MutationBaseCounts {
		totalSubstitution += counts["substitution"]
		totalInsertion += counts["insertion"]
		totalDeletion += counts["deletion"]
	}

	totalSubstitutionRate := 0.0
	totalInsertionRate := 0.0
	totalDeletionRate := 0.0

	if stats.TotalAlignedReads > 0 {
		totalSubstitutionRate = float64(totalSubstitution) / float64(stats.TotalAlignedReads) * 1000
		totalInsertionRate = float64(totalInsertion) / float64(stats.TotalAlignedReads) * 1000
		totalDeletionRate = float64(totalDeletion) / float64(stats.TotalAlignedReads) * 1000
	}

	writer.WriteString(fmt.Sprintf("Total,%d,%d,%d,%d,%d,%.4f,%.4f,%.4f\n",
		stats.TotalReadCount, stats.TotalAlignedReads,
		totalSubstitution, totalInsertion, totalDeletion,
		totalSubstitutionRate, totalInsertionRate, totalDeletionRate))

	writer.Flush()
	fmt.Printf("碱基维度变异统计已写入: %s\n", filename)

	// 2. 单碱基缺失统计
	filename = filepath.Join(outputDir, "single_base_deletion_stats.csv")
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer = bufio.NewWriter(file)
	writer.WriteString("Base,SampleCounts,TotalCount,SampleReads,TotalReads,SampleAlignedReads,TotalAlignedReads\n")

	// 碱基顺序
	bases := []byte{'A', 'C', 'G', 'T', 'N'}

	for _, base := range bases {
		totalCount := stats.TotalDeleteBaseCounts[base]

		// 各样本计数
		sampleCounts := []string{}
		sampleReads := []string{}
		sampleAlignedReads := []string{}

		for _, sampleName := range sampleNames {
			count := stats.SampleDeleteBaseCounts[sampleName][base]
			if count > 0 {
				sampleCounts = append(sampleCounts, fmt.Sprintf("%s:%d", sampleName, count))
				sampleReads = append(sampleReads, fmt.Sprintf("%s:%d", sampleName, stats.SampleReadCounts[sampleName]))
				sampleAlignedReads = append(sampleAlignedReads, fmt.Sprintf("%s:%d", sampleName, stats.SampleAlignedReads[sampleName]))
			}
		}

		writer.WriteString(fmt.Sprintf("%c,%s,%d,%s,%d,%s,%d\n",
			base,
			strings.Join(sampleCounts, ";"),
			totalCount,
			strings.Join(sampleReads, ";"),
			stats.TotalReadCount,
			strings.Join(sampleAlignedReads, ";"),
			stats.TotalAlignedReads))
	}

	writer.Flush()
	fmt.Printf("单碱基缺失统计已写入: %s\n", filename)

	// 3. 插入序列统计
	filename = filepath.Join(outputDir, "insertion_sequence_stats.csv")
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer = bufio.NewWriter(file)
	writer.WriteString("InsertionSequence,SampleCounts,TotalCount\n")

	// 收集所有插入序列
	allInsertSeqs := make(map[string]bool)
	for sampleName := range stats.SampleInsertBaseCounts {
		for seq := range stats.SampleInsertBaseCounts[sampleName] {
			allInsertSeqs[seq] = true
		}
	}

	// 转换为切片并按长度和字母排序
	var insertSeqs []string
	for seq := range allInsertSeqs {
		insertSeqs = append(insertSeqs, seq)
	}
	sort.Slice(insertSeqs, func(i, j int) bool {
		if len(insertSeqs[i]) != len(insertSeqs[j]) {
			return len(insertSeqs[i]) < len(insertSeqs[j])
		}
		return insertSeqs[i] < insertSeqs[j]
	})

	// 只输出前100个最常见的插入序列
	outputLimit := 100
	if len(insertSeqs) > outputLimit {
		insertSeqs = insertSeqs[:outputLimit]
	}

	for _, seq := range insertSeqs {
		totalCount := stats.TotalInsertBaseCounts[seq]

		// 各样本计数
		sampleCounts := []string{}
		for _, sampleName := range sampleNames {
			if counts, ok := stats.SampleInsertBaseCounts[sampleName]; ok {
				if count, exists := counts[seq]; exists && count > 0 {
					sampleCounts = append(sampleCounts, fmt.Sprintf("%s:%d", sampleName, count))
				}
			}
		}

		writer.WriteString(fmt.Sprintf("%s,%s,%d\n",
			seq,
			strings.Join(sampleCounts, ";"),
			totalCount))
	}

	writer.Flush()
	fmt.Printf("插入序列统计已写入: %s\n", filename)

	return nil
}

// writeDeletionPositionStats 写入缺失位置统计
func writeDeletionPositionStats(stats *MutationStats, outputDir string) error {
	// 1. 各样本的单碱基缺失位置统计
	for sampleName := range stats.SampleDeletePositionCounts {
		filename := filepath.Join(outputDir, fmt.Sprintf("%s_single_base_deletion_positions.csv", sampleName))
		file, err := os.Create(filename)
		if err != nil {
			return fmt.Errorf("创建文件失败 %s: %v", filename, err)
		}

		writer := bufio.NewWriter(file)
		writer.WriteString("Position,Base,Count,TotalReads,AlignedReads\n")

		// 收集位置信息并按位置排序
		type positionInfo struct {
			posKey string
			count  int
		}

		var positions []positionInfo
		for posKey, count := range stats.SampleDeletePositionCounts[sampleName] {
			positions = append(positions, positionInfo{posKey, count})
		}

		// 按位置排序
		sort.Slice(positions, func(i, j int) bool {
			// 解析位置
			posI := extractPosition(positions[i].posKey)
			posJ := extractPosition(positions[j].posKey)
			return posI < posJ
		})

		totalReads := stats.SampleReadCounts[sampleName]
		alignedReads := stats.SampleAlignedReads[sampleName]

		for _, info := range positions {
			// 解析位置和碱基
			parts := strings.Split(info.posKey, ":")
			if len(parts) == 2 {
				position := parts[0]
				base := parts[1]
				writer.WriteString(fmt.Sprintf("%s,%s,%d,%d,%d\n",
					position, base, info.count, totalReads, alignedReads))
			}
		}

		writer.Flush()
		file.Close()
		fmt.Printf("  样本 %s 的缺失位置统计已写入: %s\n", sampleName, filename)
	}

	// 2. 汇总的单碱基缺失位置统计
	filename := filepath.Join(outputDir, "total_single_base_deletion_positions.csv")
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	writer.WriteString("Position,Base,TotalCount,SampleCounts,SampleReads,SampleAlignedReads\n")

	// 收集所有位置信息
	type totalPositionInfo struct {
		posKey       string
		totalCount   int
		sampleCounts []string
	}

	var totalPositions []totalPositionInfo
	for posKey, totalCount := range stats.TotalDeletePositionCounts {
		if totalCount > 0 {
			// 收集各样本的计数
			var sampleCounts []string
			for sampleName := range stats.SampleDeletePositionCounts {
				if count, exists := stats.SampleDeletePositionCounts[sampleName][posKey]; exists && count > 0 {
					sampleCounts = append(sampleCounts, fmt.Sprintf("%s:%d", sampleName, count))
				}
			}

			totalPositions = append(totalPositions, totalPositionInfo{
				posKey:       posKey,
				totalCount:   totalCount,
				sampleCounts: sampleCounts,
			})
		}
	}

	// 按总计数降序排序
	sort.Slice(totalPositions, func(i, j int) bool {
		if totalPositions[i].totalCount != totalPositions[j].totalCount {
			return totalPositions[i].totalCount > totalPositions[j].totalCount
		}
		return totalPositions[i].posKey < totalPositions[j].posKey
	})

	// 只输出前1000个最常见的位置
	outputLimit := 1000
	if len(totalPositions) > outputLimit {
		totalPositions = totalPositions[:outputLimit]
	}

	for _, info := range totalPositions {
		// 解析位置和碱基
		parts := strings.Split(info.posKey, ":")
		if len(parts) == 2 {
			position := parts[0]
			base := parts[1]

			// 收集样本reads信息
			var sampleReadsInfo []string
			var sampleAlignedReadsInfo []string

			for sampleName := range stats.SampleDeletePositionCounts {
				if _, exists := stats.SampleDeletePositionCounts[sampleName][info.posKey]; exists {
					sampleReadsInfo = append(sampleReadsInfo,
						fmt.Sprintf("%s:%d", sampleName, stats.SampleReadCounts[sampleName]))
					sampleAlignedReadsInfo = append(sampleAlignedReadsInfo,
						fmt.Sprintf("%s:%d", sampleName, stats.SampleAlignedReads[sampleName]))
				}
			}

			writer.WriteString(fmt.Sprintf("%s,%s,%d,%s,%s,%s\n",
				position, base, info.totalCount,
				strings.Join(info.sampleCounts, ";"),
				strings.Join(sampleReadsInfo, ";"),
				strings.Join(sampleAlignedReadsInfo, ";")))
		}
	}

	writer.Flush()
	fmt.Printf("汇总缺失位置统计已写入: %s\n", filename)

	// 3. 缺失位置热力图数据
	filename = filepath.Join(outputDir, "deletion_position_heatmap.csv")
	file, err = os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败 %s: %v", filename, err)
	}
	defer file.Close()

	writer = bufio.NewWriter(file)

	// 收集所有位置
	allPositions := make(map[int]bool)
	for posKey := range stats.TotalDeletePositionCounts {
		if pos := extractPosition(posKey); pos > 0 {
			allPositions[pos] = true
		}
	}

	// 转换为排序的切片
	var positionsList []int
	for pos := range allPositions {
		positionsList = append(positionsList, pos)
	}
	sort.Ints(positionsList)

	// 写入表头
	header := "Position"
	bases := []string{"A", "C", "G", "T", "N"}
	for _, base := range bases {
		header += fmt.Sprintf(",Total_%s", base)
	}

	// 添加样本列
	var sampleNames []string
	if excelFile != "" && len(sampleOrder) > 0 {
		sampleNames = sampleOrder
	} else {
		for sampleName := range stats.SampleDeletePositionCounts {
			sampleNames = append(sampleNames, sampleName)
		}
		sort.Strings(sampleNames)
	}

	for _, sampleName := range sampleNames {
		for _, base := range bases {
			header += fmt.Sprintf(",%s_%s", sampleName, base)
		}
	}

	writer.WriteString(header + "\n")

	// 写入每行数据
	for _, position := range positionsList {
		row := fmt.Sprintf("%d", position)

		// 总计数据
		for _, base := range bases {
			posKey := fmt.Sprintf("%d:%s", position, base)
			count := stats.TotalDeletePositionCounts[posKey]
			row += fmt.Sprintf(",%d", count)
		}

		// 各样本数据
		for _, sampleName := range sampleNames {
			for _, base := range bases {
				posKey := fmt.Sprintf("%d:%s", position, base)
				count := 0
				if sampleCounts, ok := stats.SampleDeletePositionCounts[sampleName]; ok {
					count = sampleCounts[posKey]
				}
				row += fmt.Sprintf(",%d", count)
			}
		}

		writer.WriteString(row + "\n")
	}

	writer.Flush()
	fmt.Printf("缺失位置热力图数据已写入: %s\n", filename)

	return nil
}

// extractPosition 从位置键中提取位置数字
func extractPosition(posKey string) int {
	parts := strings.Split(posKey, ":")
	if len(parts) >= 1 {
		pos, err := strconv.Atoi(parts[0])
		if err == nil {
			return pos
		}
	}
	return 0
}

func mainWrite(outputDir string, stats *MutationStats) {
	// 写入各bam各位置各碱基变化组合的个数分布统计
	if err := writePositionStats(stats, outputDir); err != nil {
		fmt.Printf("写入位置统计失败: %v\n", err)
	}

	// 写入各bam各碱基变化组合的分布
	if err := writeSampleMutationStats(stats, outputDir); err != nil {
		fmt.Printf("写入样本突变统计失败: %v\n", err)
	}

	// 写入总碱基变化组合的分布
	if err := writeTotalMutationStats(stats, outputDir); err != nil {
		fmt.Printf("写入总突变统计失败: %v\n", err)
	}

	// 写入read类型统计
	if err := writeReadTypeStats(stats, outputDir); err != nil {
		fmt.Printf("写入read类型统计失败: %v\n", err)
	}
	// 写入汇总报告
	if err := writeSummaryReport(stats, outputDir); err != nil {
		fmt.Printf("写入汇总报告失败: %v\n", err)
	}

	// 新增：写入详细统计
	if err := writeDetailedStats(stats, outputDir); err != nil {
		fmt.Printf("写入详细统计失败: %v\n", err)
	}

	// 新增：写入碱基维度统计
	if err := writeBaseMutationStats(stats, outputDir); err != nil {
		fmt.Printf("写入碱基维度统计失败: %v\n", err)
	}

	// 新增：写入缺失位置统计
	if err := writeDeletionPositionStats(stats, outputDir); err != nil {
		fmt.Printf("写入缺失位置统计失败: %v\n", err)
	}
}

func mainPrint(stats *MutationStats) {
	// 打印总体统计
	fmt.Printf("\n处理完成!\n")
	fmt.Printf("总体统计:\n")
	fmt.Printf("  总reads数: %d\n", stats.TotalReadCount)
	fmt.Printf("  总包含突变reads数: %d\n", stats.TotalReadsWithMuts)
	fmt.Printf("  Read类型分布:\n")
	for rt := ReadTypeMatch; rt <= ReadTypeAll; rt++ {
		count := stats.TotalReadTypeCounts[rt]
		if count > 0 {
			percentage := float64(count) / float64(stats.TotalReadCount) * 100
			fmt.Printf("    %s: %d (%.2f%%)\n", ReadTypeNames[rt], count, percentage)
		}
	}
}
