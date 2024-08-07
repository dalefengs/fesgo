package orm

import (
	"database/sql"
	"errors"
	"fmt"
	fesLog "github.com/dalefeng/fesgo/logger"
	_ "github.com/go-sql-driver/mysql" // This allows you to use mysql with the sql package.
	"reflect"
	"strings"
)

type FesDB struct {
	db     *sql.DB
	logger *fesLog.Logger
	Prefix string
}

type FesSession struct {
	db          *FesDB
	tableName   string
	FieldName   []string
	PlaceHolder []string
	values      []interface{}
	UpdateParam strings.Builder
	WhereParam  strings.Builder
	WhereValue  []interface{}
}

func Open(driverName, dataSourceName string) *FesDB {
	db, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		panic(err)
	}
	// 最大连接数
	db.SetMaxOpenConns(100)
	// 最大空闲连接数
	db.SetMaxIdleConns(20)
	// 连接最大存活时间
	db.SetConnMaxLifetime(60)
	// 连接最大空闲时间
	db.SetConnMaxIdleTime(30)

	err = db.Ping()
	if err != nil {
		panic(err)
	}
	return &FesDB{db: db, logger: fesLog.Default()}
}

func (db *FesDB) NewSession(data any) *FesSession {
	m := &FesSession{db: db}
	t := reflect.TypeOf(data)

	tVar := t.Elem()
	if t.Kind() == reflect.Pointer {
		tVar = t.Elem()
	}

	if m.tableName == "" {
		m.tableName = m.db.Prefix + strings.ToLower(Name(tVar.Name()))
	}
	return m

}
func (s *FesSession) Table(name string) *FesSession {
	s.tableName = s.db.Prefix + name
	return s
}

func (s *FesSession) Insert(data any) (int64, int64, error) {
	s.fieldNames(data)
	query := fmt.Sprintf("insert into %s (%s) values(%s)", s.tableName, strings.Join(s.FieldName, ","), strings.Join(s.PlaceHolder, ","))
	s.db.logger.Info("sql", query)
	stmt, err := s.db.db.Prepare(query)
	if err != nil {
		return -1, -1, err
	}
	res, err := stmt.Exec(s.values...)
	if err != nil {
		return -1, -1, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return -1, -1, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return -1, -1, err
	}
	return id, affected, nil
}

func (s *FesSession) InsertBatch(data []any) (int64, int64, error) {
	if len(data) == 0 {
		return -1, -1, errors.New("no data insert")
	}
	s.fieldNames(data[0])
	query := fmt.Sprintf("insert into %s (%s) values", s.tableName, strings.Join(s.FieldName, ","))

	var sb strings.Builder
	sb.WriteString(query)

	for index, _ := range data {
		sb.WriteString("(")
		sb.WriteString(strings.Join(s.PlaceHolder, ","))
		sb.WriteString(")")
		if index < len(data)-1 {
			sb.WriteString(",")
		}
	}
	s.batchValues(data)

	s.db.logger.Info("sql", sb.String())
	stmt, err := s.db.db.Prepare(sb.String())
	if err != nil {
		return -1, -1, err
	}
	res, err := stmt.Exec(s.values...)
	if err != nil {
		return -1, -1, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return -1, -1, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return -1, -1, err
	}
	return id, affected, nil
}

func (s *FesSession) Update(data ...any) (int64, int64, error) {

	if len(data) == 00 || len(data) > 2 {
		return -1, -1, errors.New("param not valid")
	}

	single := true
	if len(data) == 2 {
		single = false
	}

	// //update table set age=?,name=? where id=?
	if !single {
		if s.UpdateParam.String() != "" {
			s.UpdateParam.WriteString(",")
		}
		s.UpdateParam.WriteString(data[0].(string))
		s.UpdateParam.WriteString(" = ?")
		s.values = append(s.values, data[1])
	} else {
		updateData := data[0]
		t := reflect.TypeOf(updateData)
		v := reflect.ValueOf(updateData)
		if t.Kind() != reflect.Pointer {
			panic("updateData must be a struct")
		}

		tVar := t.Elem()
		vVar := v.Elem()
		if s.tableName == "" {
			s.tableName = s.db.Prefix + strings.ToLower(Name(tVar.Name()))
		}
		for i := 0; i < tVar.NumField(); i++ {
			field := tVar.Field(i)

			tag := field.Tag
			sqlTag := tag.Get("form")
			if sqlTag == "" {
				sqlTag = strings.ToLower(Name(field.Name))
			} else {
				if strings.Contains(sqlTag, "auto_increment") {
					continue
				}
				if strings.Contains(sqlTag, ",") {
					sqlTag = sqlTag[:strings.Index(sqlTag, ",")]
				}
			}

			val := vVar.Field(i).Interface()
			if strings.ToLower(sqlTag) == "id" && IsAutoId(val) {
				continue
			}
			if s.UpdateParam.String() != "" {
				s.UpdateParam.WriteString(",")
			}
			s.UpdateParam.WriteString(data[0].(string))
			s.UpdateParam.WriteString(" = ?")
			s.values = append(s.values, val)
		}

	}
	query := fmt.Sprintf("update %s set %s", s.tableName, s.UpdateParam.String())
	var sb strings.Builder
	sb.WriteString(query)
	sb.WriteString(s.WhereParam.String())

	s.db.logger.Info("sql", sb.String())
	stmt, err := s.db.db.Prepare(sb.String())
	if err != nil {
		return -1, -1, err
	}
	s.values = append(s.values, s.WhereValue...)
	res, err := stmt.Exec(s.values...)
	if err != nil {
		return -1, -1, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return -1, -1, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return -1, -1, err
	}
	return id, affected, nil
}

func (s *FesSession) Delete(data any) (int64, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("delete from %s", s.tableName))
	sb.WriteString(s.WhereParam.String())
	s.db.logger.Info(sb.String())

	stmt, err := s.db.db.Prepare(sb.String())
	if err != nil {
		return 0, err
	}
	r, err := stmt.Exec(s.WhereValue...)
	if err != nil {
		return 0, err
	}

	return r.RowsAffected()

}

func (s *FesSession) Select(data any, fields ...string) ([]any, error) {
	t := reflect.TypeOf(data)
	if t.Kind() != reflect.Pointer {
		return nil, errors.New("data must ne pointer")
	}
	var sb strings.Builder
	fieldStr := "*"
	if len(fields) > 0 {
		fieldStr = strings.Join(fields, ",")
	}
	sb.WriteString(fmt.Sprintf("select %s from %s", fieldStr, s.tableName))
	sb.WriteString(s.WhereParam.String())
	s.db.logger.Info(sb.String())

	prepare, err := s.db.db.Prepare(sb.String())
	if err != nil {
		return nil, err
	}
	rows, err := prepare.Query(s.WhereValue...)
	if err != nil {
		return nil, err
	}
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	result := make([]any, 0)
	for rows.Next() {
		data = reflect.New(t.Elem()).Interface()
		values := make([]any, len(columns))
		fieldScan := make([]any, len(columns))

		for i := range fieldScan {
			fieldScan[i] = &values[i]
		}
		err = rows.Scan(fieldScan...)
		if err != nil {
			return nil, err
		}
		tVar := t.Elem()
		tVal := reflect.ValueOf(data).Elem()
		for i := 0; i < tVar.NumField(); i++ {
			name := tVar.Field(i).Name
			tag := tVar.Field(i).Tag
			sqlTag := tag.Get("fesgo")
			if sqlTag == "" {
				sqlTag = strings.ToLower(Name(name))
			} else {
				if strings.Contains(sqlTag, ",") {
					sqlTag = sqlTag[:strings.Index(sqlTag, ",")]
				}
			}
			for j, colName := range columns {
				if sqlTag == colName {
					target := reflect.ValueOf(values[j]).Interface()
					fieldType := tVar.Field(i).Type
					result := reflect.ValueOf(target).Convert(fieldType)
					tVal.Field(i).Set(result)
				}
			}
		}
		result = append(result, data)
	}
	return result, nil
}

func (s *FesSession) SelectOne(data any, fields ...string) error {
	t := reflect.TypeOf(data)
	if t.Kind() != reflect.Pointer {
		return errors.New("data must ne pointer")
	}
	var sb strings.Builder
	fieldStr := "*"
	if len(fields) > 0 {
		fieldStr = strings.Join(fields, ",")
	}
	sb.WriteString(fmt.Sprintf("select %s from %s", fieldStr, s.tableName))
	sb.WriteString(s.WhereParam.String())
	s.db.logger.Info(sb.String())

	prepare, err := s.db.db.Prepare(sb.String())
	if err != nil {
		return err
	}
	rows, err := prepare.Query(s.WhereValue...)
	if err != nil {
		return err
	}
	columns, err := rows.Columns()
	if err != nil {
		return err
	}
	values := make([]any, len(columns))
	fieldScan := make([]any, len(columns))

	for i := range fieldScan {
		fieldScan[i] = &values[i]
	}
	if rows.Next() {
		err = rows.Scan(fieldScan...)
		if err != nil {
			return err
		}
		tVar := t.Elem()
		tVal := reflect.ValueOf(data).Elem()
		for i := 0; i < tVar.NumField(); i++ {
			name := tVar.Field(i).Name
			tag := tVar.Field(i).Tag
			sqlTag := tag.Get("fesgo")
			if sqlTag == "" {
				sqlTag = strings.ToLower(Name(name))
			} else {
				if strings.Contains(sqlTag, ",") {
					sqlTag = sqlTag[:strings.Index(sqlTag, ",")]
				}
			}
			for j, colName := range columns {
				if sqlTag == colName {
					target := reflect.ValueOf(values[j]).Interface()
					fieldType := tVar.Field(i).Type
					result := reflect.ValueOf(target).Convert(fieldType)
					tVal.Field(i).Set(result)
				}
			}
		}

	}
	return nil
}

func (s *FesSession) Where(field string, value any) *FesSession {
	if s.WhereParam.String() == "" {
		s.WhereParam.WriteString(" where ")
	}
	s.WhereParam.WriteString(field)
	s.WhereParam.WriteString(" = ")
	s.WhereParam.WriteString(" ? ")
	s.WhereValue = append(s.WhereValue, value)
	return s
}

func (s *FesSession) And(field string, value any) *FesSession {
	s.WhereParam.WriteString(" and ")
	return s

}

func (s *FesSession) Or(field string, value any) *FesSession {
	s.WhereParam.WriteString(" or ")
	return s
}

func (s *FesSession) Exec(sql string, values ...any) (int64, error) {
	stmt, err := s.db.db.Prepare(sql)
	if err != nil {
		return 0, err
	}
	exec, err := stmt.Exec(values)
	if err != nil {
		return 0, err
	}
	if strings.Contains(strings.ToLower(sql), "insert") {
		return exec.LastInsertId()
	}
	return exec.RowsAffected()
}

func (s *FesSession) QueryRaw(sql string, data any, queryValues ...any) error {
	t := reflect.TypeOf(data)
	if t.Kind() != reflect.Pointer {
		return errors.New("data must be pointer")
	}
	stmt, err := s.db.db.Prepare(sql)
	if err != nil {
		return err
	}
	rows, err := stmt.Query(queryValues)
	if err != nil {
		return err
	}

	columns, err := rows.Columns()
	if err != nil {
		return err
	}
	values := make([]any, len(columns))
	fieldScan := make([]any, len(columns))

	for i := range fieldScan {
		fieldScan[i] = &values[i]
	}
	if rows.Next() {
		err = rows.Scan(fieldScan...)
		if err != nil {
			return err
		}
		tVar := t.Elem()
		tVal := reflect.ValueOf(data).Elem()
		for i := 0; i < tVar.NumField(); i++ {
			name := tVar.Field(i).Name
			tag := tVar.Field(i).Tag
			sqlTag := tag.Get("fesgo")
			if sqlTag == "" {
				sqlTag = strings.ToLower(Name(name))
			} else {
				if strings.Contains(sqlTag, ",") {
					sqlTag = sqlTag[:strings.Index(sqlTag, ",")]
				}
			}
			for j, colName := range columns {
				if sqlTag == colName {
					target := reflect.ValueOf(values[j]).Interface()
					fieldType := tVar.Field(i).Type
					result := reflect.ValueOf(target).Convert(fieldType)
					tVal.Field(i).Set(result)
				}
			}
		}

	}
	return nil
}

func (s *FesSession) fieldNames(data any) {
	t := reflect.TypeOf(data)
	v := reflect.ValueOf(data)
	if t.Kind() != reflect.Pointer {
		panic("data must be a struct")
	}

	tVar := t.Elem()
	vVar := v.Elem()
	if s.tableName == "" {
		s.tableName = s.db.Prefix + strings.ToLower(Name(tVar.Name()))
	}
	for i := 0; i < tVar.NumField(); i++ {
		field := tVar.Field(i)

		tag := field.Tag
		sqlTag := tag.Get("form")
		if sqlTag == "" {
			sqlTag = strings.ToLower(Name(field.Name))
		} else {
			if strings.Contains(sqlTag, "auto_increment") {
				continue
			}
			if strings.Contains(sqlTag, ",") {
				sqlTag = sqlTag[:strings.Index(sqlTag, ",")]
			}
		}

		val := vVar.Field(i).Interface()
		if strings.ToLower(sqlTag) == "id" && IsAutoId(val) {
			continue
		}
		s.FieldName = append(s.FieldName, sqlTag)
		s.PlaceHolder = append(s.PlaceHolder, "?")
		s.values = append(s.values, val)
	}
}

func (s *FesSession) batchValues(data []any) {
	s.values = make([]any, 0, len(data))
	for _, d := range data {
		t := reflect.TypeOf(d)
		v := reflect.ValueOf(d)
		if t.Kind() != reflect.Pointer {
			panic("data must be a struct")
		}

		tVar := t.Elem()
		vVar := v.Elem()
		for i := 0; i < tVar.NumField(); i++ {
			field := tVar.Field(i)
			tag := field.Tag
			sqlTag := tag.Get("form")
			if sqlTag == "" {
				sqlTag = strings.ToLower(Name(field.Name))
			} else {
				if strings.Contains(sqlTag, "auto_increment") {
					continue
				}
			}

			val := vVar.Field(i).Interface()
			if strings.ToLower(sqlTag) == "id" && IsAutoId(val) {
				continue
			}
			s.values = append(s.values, val)
		}
	}
}

func Name(name string) string {
	var names = name[:]
	lastIndex := 0
	var sb strings.Builder
	for index, value := range names {
		// 大写字母
		if value >= 65 && value <= 90 {
			if index == 0 {
				continue
			}
			sb.WriteString(name[:index])
			sb.WriteString("_")
			lastIndex = index
		}
	}

	sb.WriteString(names[lastIndex:])
	return sb.String()
}

func IsAutoId(id any) bool {
	t := reflect.TypeOf(id)
	switch t.Kind() {
	case reflect.Int64:
		if (id.(int64)) <= 0 {
			return true
		}
	case reflect.Int32:
		if (id.(int32)) <= 0 {
			return true
		}
	case reflect.Int:
		if (id.(int)) <= 0 {
			return true
		}
	case reflect.Int8:
		if (id.(int8)) <= 0 {
			return true
		}

	}
	return false
}

func (db *FesDB) Close() error {
	return db.db.Close()
}
