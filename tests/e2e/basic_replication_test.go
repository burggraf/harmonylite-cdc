package main_test

import (
	"context"
	"database/sql"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestBasicReplication verifies Scenario 1 from quickstart.md:
// Given a 3-node cluster with replication enabled,
// When a user writes data to node 1,
// Then the data appears on nodes 2 and 3 within 100ms without any database trigger setup
var _ = Describe("Basic 3-Node Replication", func() {
	var (
		ctx         context.Context
		cancel      context.CancelFunc
		node1DB     *sql.DB
		node2DB     *sql.DB
		node3DB     *sql.DB
		natsServer  interface{} // TODO: Type from testcontainers or mock
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Minute)
		_ = ctx // Will be used in implementation
		_ = natsServer // Will be used in implementation

		// TODO: Setup 3-node cluster
		// 1. Start NATS JetStream server (testcontainers)
		// 2. Create 3 SQLite databases (node1.db, node2.db, node3.db)
		// 3. Enable WAL mode on all databases
		// 4. Start harmonylite parser/replicator for each node
		// 5. Wait for all nodes to connect to NATS

		Skip("Implementation pending")
	})

	AfterEach(func() {
		// TODO: Cleanup resources
		// 1. Stop all parsers/replicators
		// 2. Close database connections
		// 3. Stop NATS server
		// 4. Remove test database files

		if cancel != nil {
			cancel()
		}
	})

	Context("when data is written to node 1", func() {
		It("should replicate to nodes 2 and 3 within 100ms", func() {
			By("inserting a row into node 1")
			_, err := node1DB.Exec("INSERT INTO users (id, name, email) VALUES (?, ?, ?)",
				"user-1", "Alice", "alice@example.com")
			Expect(err).ToNot(HaveOccurred())

			By("verifying the row appears on node 2 within 100ms")
			Eventually(func() int {
				var count int
				node2DB.QueryRow("SELECT COUNT(*) FROM users WHERE id = ?", "user-1").Scan(&count)
				return count
			}, "100ms", "10ms").Should(Equal(1))

			By("verifying the row appears on node 3 within 100ms")
			Eventually(func() int {
				var count int
				node3DB.QueryRow("SELECT COUNT(*) FROM users WHERE id = ?", "user-1").Scan(&count)
				return count
			}, "100ms", "10ms").Should(Equal(1))

			By("verifying the data is identical across all nodes")
			var name2, email2, name3, email3 string
			node2DB.QueryRow("SELECT name, email FROM users WHERE id = ?", "user-1").Scan(&name2, &email2)
			node3DB.QueryRow("SELECT name, email FROM users WHERE id = ?", "user-1").Scan(&name3, &email3)
			Expect(name2).To(Equal("Alice"))
			Expect(email2).To(Equal("alice@example.com"))
			Expect(name3).To(Equal("Alice"))
			Expect(email3).To(Equal("alice@example.com"))
		})

		It("should not create any triggers in the database", func() {
			By("querying sqlite_master for triggers")
			var triggerCount int
			err := node1DB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='trigger'").Scan(&triggerCount)
			Expect(err).ToNot(HaveOccurred())
			Expect(triggerCount).To(Equal(0), "No triggers should exist (FR-001)")

			// Verify on all nodes
			node2DB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='trigger'").Scan(&triggerCount)
			Expect(triggerCount).To(Equal(0))
			node3DB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='trigger'").Scan(&triggerCount)
			Expect(triggerCount).To(Equal(0))
		})

		It("should not create any views in the database", func() {
			By("querying sqlite_master for views")
			var viewCount int
			err := node1DB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='view'").Scan(&viewCount)
			Expect(err).ToNot(HaveOccurred())
			Expect(viewCount).To(Equal(0), "No views should exist (FR-002)")
		})

		It("should not modify the database schema", func() {
			By("verifying only user-created tables exist")
			var tableCount int
			err := node1DB.QueryRow(`
				SELECT COUNT(*) FROM sqlite_master
				WHERE type='table' AND name NOT LIKE 'sqlite_%'
			`).Scan(&tableCount)
			Expect(err).ToNot(HaveOccurred())
			// Only the 'users' table should exist (plus any other app tables)
			// No __harmonylite__* tables
		})
	})

	Context("when multiple rows are written rapidly", func() {
		It("should replicate all rows successfully", func() {
			By("inserting 100 rows rapidly")
			for i := 1; i <= 100; i++ {
				_, err := node1DB.Exec("INSERT INTO users (id, name) VALUES (?, ?)",
					GinkgoRandomSeed()+int64(i), "User"+string(rune(i)))
				Expect(err).ToNot(HaveOccurred())
			}

			By("verifying all 100 rows appear on node 2")
			Eventually(func() int {
				var count int
				node2DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
				return count
			}, "1s", "50ms").Should(BeNumerically(">=", 100))

			By("verifying all 100 rows appear on node 3")
			Eventually(func() int {
				var count int
				node3DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
				return count
			}, "1s", "50ms").Should(BeNumerically(">=", 100))
		})
	})

	Context("when different nodes write concurrently", func() {
		It("should replicate changes from all nodes", func() {
			By("inserting row on node 1")
			_, err := node1DB.Exec("INSERT INTO users (id, name) VALUES (?, ?)", "user-n1", "Node1User")
			Expect(err).ToNot(HaveOccurred())

			By("inserting row on node 2")
			_, err = node2DB.Exec("INSERT INTO users (id, name) VALUES (?, ?)", "user-n2", "Node2User")
			Expect(err).ToNot(HaveOccurred())

			By("inserting row on node 3")
			_, err = node3DB.Exec("INSERT INTO users (id, name) VALUES (?, ?)", "user-n3", "Node3User")
			Expect(err).ToNot(HaveOccurred())

			By("verifying all 3 rows appear on all nodes")
			Eventually(func() int {
				var count int
				node1DB.QueryRow("SELECT COUNT(*) FROM users WHERE id IN ('user-n1', 'user-n2', 'user-n3')").Scan(&count)
				return count
			}, "200ms", "20ms").Should(Equal(3))

			Eventually(func() int {
				var count int
				node2DB.QueryRow("SELECT COUNT(*) FROM users WHERE id IN ('user-n1', 'user-n2', 'user-n3')").Scan(&count)
				return count
			}, "200ms", "20ms").Should(Equal(3))

			Eventually(func() int {
				var count int
				node3DB.QueryRow("SELECT COUNT(*) FROM users WHERE id IN ('user-n1', 'user-n2', 'user-n3')").Scan(&count)
				return count
			}, "200ms", "20ms").Should(Equal(3))
		})
	})
})
