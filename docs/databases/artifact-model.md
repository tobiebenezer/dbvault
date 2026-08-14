# Phase 4 Artifact Model

A snapshot may contain multiple required artifacts. PostgreSQL can include a `globals.sql` artifact and a main database archive. MySQL/MariaDB normally produce one SQL artifact.

Each artifact has its own root digest. The backup-set digest covers ordered required artifacts, restore order and engine metadata. Retention and GC treat a backup set atomically.
