package slicesx

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

func Int64SliceToStringSlice(slice []int64) []string {
	stringSlice := make([]string, len(slice))
	for i, num := range slice {
		stringSlice[i] = strconv.FormatInt(num, 10)
	}
	return stringSlice
}

// ParseInt64CSV parses a comma-separated list of integers, such as the id
// lists Int64SliceToString stores ("1,2,3"). Blank input is an empty (nil)
// list and space around an element is ignored. Any element that is not an
// integer, an empty one included, fails the whole list: a damaged value is
// never mistaken for a shorter list — for a coupon's plan list, a shorter or
// empty list would widen what the coupon applies to.
func ParseInt64CSV(s string) ([]int64, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	elements := strings.Split(s, ",")
	ids := make([]int64, 0, len(elements))
	for i, element := range elements {
		id, err := strconv.ParseInt(strings.TrimSpace(element), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("element %d of the id list %q: %w", i+1, s, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func Int64SliceToString(intSlice []int64) string {
	var strSlice []string
	for _, num := range intSlice {
		strSlice = append(strSlice, strconv.FormatInt(num, 10))
	}
	return strings.Join(strSlice, ",")
}

// string slice to string
func StringSliceToString(stringSlice []string) string {
	stringSlice = RemoveDuplicateElements(stringSlice...)
	return strings.Join(stringSlice, ",")
}

// StringMergeAndRemoveDuplicates Tool function to convert multiple comma separated strings into [] strings and deduplicate them
func StringMergeAndRemoveDuplicates(strs ...string) []string {
	if len(strs) == 1 && strs[0] == "" {
		return []string{}
	}
	merged := make([]string, 0)
	for _, str := range strs {
		merged = append(merged, strings.Split(str, ",")...)
	}
	uniqueMap := make(map[string]bool)
	var uniqueList []string

	for _, item := range merged {
		if !uniqueMap[item] {
			uniqueMap[item] = true
			uniqueList = append(uniqueList, item)
		}
	}

	return uniqueList
}

func RemoveDuplicateElements[T comparable](input ...T) []T {
	uniqueMap := make(map[T]struct{})
	var result []T

	for _, item := range input {
		// 仅在 T 是 string 类型时跳过空字符串
		if v, ok := any(item).(string); ok && v == "" {
			continue
		}
		if _, exists := uniqueMap[item]; !exists {
			uniqueMap[item] = struct{}{}
			result = append(result, item)
		}
	}
	return result
}

// RemoveStringElement 移除指定元素
func RemoveStringElement(arr []string, element ...string) []string {
	var result []string
	for _, str := range arr {
		if !slices.Contains(element, str) {
			result = append(result, str)
		}
	}
	return result
}
