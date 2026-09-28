package mapping

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/jinzhu/copier"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/pkg/errors"
)

// CopyOption 定义复制选项的函数类型
type CopyOption func(*copier.Option)

// CopyWithIgnoreEmpty 设置是否忽略空值
func CopyWithIgnoreEmpty(ignoreEmpty bool) CopyOption {
	return func(o *copier.Option) {
		o.IgnoreEmpty = ignoreEmpty
	}
}

func DeepCopy[T, K any](destStruct T, srcStruct K, opts ...CopyOption) T {
	var dst = destStruct
	var src = srcStruct

	option := copier.Option{
		DeepCopy:    true,
		IgnoreEmpty: false,
		Converters: []copier.TypeConverter{
			{
				SrcType: time.Time{},
				DstType: int64(0),
				Fn: func(src any) (any, error) {
					s, ok := src.(time.Time)
					if !ok {
						return nil, errors.New("src type not matching")
					}
					return s.UnixMilli(), nil
				},
			},
		},
	}

	for _, opt := range opts {
		opt(&option)
	}

	if err := copier.CopyWithOption(dst, src, option); err != nil {
		// A failed copy leaves dst partly filled; it is a programming error
		// (mismatched types), so it must at least be visible.
		logger.Errorw("[mapping] DeepCopy failed", logger.Field("error", err.Error()),
			logger.Field("dst_type", fmt.Sprintf("%T", destStruct)), logger.Field("src_type", fmt.Sprintf("%T", srcStruct)))
	}
	return dst
}

func CloneMapToStruct(input any, output interface{}) error {
	// 确保 output 是一个指针，并且指向一个结构体
	val := reflect.ValueOf(output)
	if val.Kind() != reflect.Ptr || val.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("output must be a pointer to a struct")
	}

	// 使用 JSON 编解码将 map 转换为结构体
	data, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("failed to marshal input: %w", err)
	}

	err = json.Unmarshal(data, output)
	if err != nil {
		return fmt.Errorf("failed to unmarshal data to struct: %w", err)
	}
	return nil
}
