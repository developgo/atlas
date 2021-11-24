// Copyright 2021-present The Atlas Authors. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package integration

import (
	"context"
	"database/sql"
	"testing"

	"ariga.io/atlas/sql/postgres"
	"ariga.io/atlas/sql/schema"
	"ariga.io/atlas/sql/sqlite"

	"entgo.io/ent/dialect"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

type liteTest struct {
	*testing.T
	db  *sql.DB
	drv *sqlite.Driver
}

func liteRun(t *testing.T, fn func(test *liteTest)) {
	db, err := sql.Open("sqlite3", "file:atlas?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	drv, err := sqlite.Open(db)
	require.NoError(t, err)
	tt := &liteTest{T: t, db: db, drv: drv}
	fn(tt)
}

func TestSQLite_AddDropTable(t *testing.T) {
	liteRun(t, func(t *liteTest) {
		testAddDrop(t)
	})
}

func TestSQLite_Relation(t *testing.T) {
	liteRun(t, func(t *liteTest) {
		testRelation(t)
	})
}

func TestSQLite_Ent(t *testing.T) {
	liteRun(t, func(t *liteTest) {
		testEntIntegration(t, dialect.SQLite, t.db)
	})
}

func TestSQLite_AddIndexedColumns(t *testing.T) {
	liteRun(t, func(t *liteTest) {
		usersT := &schema.Table{
			Name:    "users",
			Columns: []*schema.Column{{Name: "id", Type: &schema.ColumnType{Type: &schema.IntegerType{T: "int"}}}},
		}
		t.migrate(&schema.AddTable{T: usersT})
		t.dropTables(usersT.Name)

		// Insert 2 records to the users table, and make sure they are there
		// after executing migration.
		_, err := t.db.Exec("INSERT INTO users (id) VALUES (1), (2)")
		require.NoError(t, err)

		usersT.Columns = append(usersT.Columns, &schema.Column{
			Name:    "a",
			Type:    &schema.ColumnType{Type: &schema.IntegerType{T: "integer"}, Null: true},
			Default: &schema.RawExpr{X: "10"},
		}, &schema.Column{
			Name:    "b",
			Type:    &schema.ColumnType{Type: &schema.IntegerType{T: "integer"}, Null: true},
			Default: &schema.RawExpr{X: "20"},
		}, &schema.Column{
			Name:    "c",
			Type:    &schema.ColumnType{Type: &schema.IntegerType{T: "integer"}, Null: true},
			Default: &schema.RawExpr{X: "30"},
		})
		usersT.Indexes = append(usersT.Indexes, &schema.Index{
			Unique: true,
			Name:   "id_a_b_c_unique",
			Parts:  []*schema.IndexPart{{C: usersT.Columns[0]}, {C: usersT.Columns[1]}, {C: usersT.Columns[2]}, {C: usersT.Columns[3]}},
		})
		changes := t.diff(t.loadUsers(), usersT)
		require.Len(t, changes, 4, "usersT contains 3 new columns and 1 new index")
		t.migrate(&schema.ModifyTable{T: usersT, Changes: changes})
		ensureNoChange(t, usersT)

		// Scan records from the table to ensure correctness of
		// the rows transferring.
		rows, err := t.db.Query("SELECT * FROM users")
		require.NoError(t, err)
		require.True(t, rows.Next())
		var v [4]int
		require.NoError(t, rows.Scan(&v[0], &v[1], &v[2], &v[3]))
		require.Equal(t, [4]int{1, 10, 20, 30}, v)
		require.True(t, rows.Next())
		require.NoError(t, rows.Scan(&v[0], &v[1], &v[2], &v[3]))
		require.Equal(t, [4]int{2, 10, 20, 30}, v)
		require.False(t, rows.Next())
		require.NoError(t, rows.Close())

		// Dropping a column from both table and index.
		usersT = t.loadUsers()
		idx, ok := usersT.Index("id_a_b_c_unique")
		require.True(t, ok)
		require.Len(t, idx.Parts, 4)
		usersT.Columns = usersT.Columns[:len(usersT.Columns)-1]
		idx.Parts = idx.Parts[:len(idx.Parts)-1]
		changes = t.diff(t.loadUsers(), usersT)
		require.Len(t, changes, 2)
		t.migrate(&schema.ModifyTable{T: usersT, Changes: changes})
		ensureNoChange(t, t.loadUsers())

		// Scan records from the table to ensure correctness of
		// the rows transferring.
		rows, err = t.db.Query("SELECT * FROM users")
		require.NoError(t, err)
		require.True(t, rows.Next())
		var u [3]int
		require.NoError(t, rows.Scan(&u[0], &u[1], &u[2]))
		require.Equal(t, [3]int{1, 10, 20}, u)
		require.True(t, rows.Next())
		require.NoError(t, rows.Scan(&u[0], &u[1], &u[2]))
		require.Equal(t, [3]int{2, 10, 20}, u)
		require.False(t, rows.Next())
		require.NoError(t, rows.Close())

	})
}

func (t *liteTest) loadRealm() *schema.Realm {
	r, err := t.drv.InspectRealm(context.Background(), &schema.InspectRealmOption{
		Schemas: []string{"main"},
	})
	require.NoError(t, err)
	return r
}

func (t *liteTest) loadUsers() *schema.Table {
	realm := t.loadRealm()
	require.Len(t, realm.Schemas, 1)
	users, ok := realm.Schemas[0].Table("users")
	require.True(t, ok)
	return users
}

func (t *liteTest) loadPosts() *schema.Table {
	realm := t.loadRealm()
	require.Len(t, realm.Schemas, 1)
	posts, ok := realm.Schemas[0].Table("posts")
	require.True(t, ok)
	return posts
}

func (t *liteTest) users() *schema.Table {
	usersT := &schema.Table{
		Name:   "users",
		Schema: t.realm().Schemas[0],
		Columns: []*schema.Column{
			{
				Name:  "id",
				Type:  &schema.ColumnType{Type: &schema.IntegerType{T: "bigint"}},
				Attrs: []schema.Attr{&postgres.Identity{}},
			},
			{
				Name: "x",
				Type: &schema.ColumnType{Type: &schema.IntegerType{T: "integer"}},
			},
		},
	}
	usersT.PrimaryKey = &schema.Index{Parts: []*schema.IndexPart{{C: usersT.Columns[0]}}}
	return usersT
}

func (t *liteTest) posts() *schema.Table {
	usersT := t.users()
	postsT := &schema.Table{
		Name:   "posts",
		Schema: t.realm().Schemas[0],
		Columns: []*schema.Column{
			{
				Name:  "id",
				Type:  &schema.ColumnType{Type: &schema.IntegerType{T: "bigint"}},
				Attrs: []schema.Attr{&postgres.Identity{}},
			},
			{
				Name:    "author_id",
				Type:    &schema.ColumnType{Type: &schema.IntegerType{T: "integer"}, Null: true},
				Default: &schema.RawExpr{X: "10"},
			},
			{
				Name: "ctime",
				Type: &schema.ColumnType{Raw: "timestamp", Type: &schema.TimeType{T: "timestamp"}},
				Default: &schema.RawExpr{
					X: "CURRENT_TIMESTAMP",
				},
			},
		},
		Attrs: []schema.Attr{
			&schema.Comment{Text: "posts comment"},
		},
	}
	postsT.PrimaryKey = &schema.Index{Parts: []*schema.IndexPart{{C: postsT.Columns[0]}}}
	postsT.Indexes = []*schema.Index{
		{Name: "author_id", Parts: []*schema.IndexPart{{C: postsT.Columns[1]}}},
		{Name: "id_author_id_unique", Unique: true, Parts: []*schema.IndexPart{{C: postsT.Columns[1]}, {C: postsT.Columns[0]}}},
	}
	postsT.ForeignKeys = []*schema.ForeignKey{
		{Symbol: "author_id", Table: postsT, Columns: postsT.Columns[1:2], RefTable: usersT, RefColumns: usersT.Columns[:1], OnDelete: schema.NoAction},
	}
	return postsT
}

func (t *liteTest) realm() *schema.Realm {
	r := &schema.Realm{
		Schemas: []*schema.Schema{
			{
				Name: "main",
				Attrs: []schema.Attr{
					&sqlite.File{Name: ":memory:"},
				},
			},
		},
	}
	r.Schemas[0].Realm = r
	return r
}

func (t *liteTest) diff(t1, t2 *schema.Table) []schema.Change {
	changes, err := t.drv.Diff().TableDiff(t1, t2)
	require.NoError(t, err)
	return changes
}

func (t *liteTest) migrate(changes ...schema.Change) {
	err := t.drv.Migrate().Exec(context.Background(), changes)
	require.NoError(t, err)
}

func (t *liteTest) dropTables(names ...string) {
	t.Cleanup(func() {
		for i := range names {
			_, err := t.db.Exec("DROP TABLE IF EXISTS " + names[i])
			require.NoError(t.T, err, "drop tables %q", names[i])
		}
	})
}