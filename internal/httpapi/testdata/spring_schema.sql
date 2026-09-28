-- Minimal test fixture matching the Spring entities and NestJS shared schema.
-- Only used inside a uniquely named integration-test schema, never by the app.
CREATE TABLE "user" (
 id SERIAL PRIMARY KEY, email TEXT NOT NULL UNIQUE, "passwordHash" TEXT NOT NULL,
 role TEXT NOT NULL, "createdAt" TIMESTAMPTZ DEFAULT now(), "updatedAt" TIMESTAMPTZ DEFAULT now()
);
CREATE TABLE department (id SERIAL PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE employee (
 id SERIAL PRIMARY KEY, "userId" INTEGER NOT NULL UNIQUE REFERENCES "user"(id),
 "employeeNumber" TEXT NOT NULL UNIQUE, "firstName" TEXT NOT NULL, "lastName" TEXT,
 "departmentId" INTEGER REFERENCES department(id), position TEXT,
 "isActive" BOOLEAN NOT NULL DEFAULT true
);
CREATE TABLE "attendanceRecord" (
 id SERIAL PRIMARY KEY, "employeeId" INTEGER NOT NULL REFERENCES employee(id),
 "attendanceDate" DATE NOT NULL, "checkIn" TIMESTAMPTZ, "checkOut" TIMESTAMPTZ,
 "createdAt" TIMESTAMPTZ NOT NULL DEFAULT now(), "updatedAt" TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE ("employeeId","attendanceDate")
);
INSERT INTO "user"(email,"passwordHash",role) VALUES
 ('employee@example.com','unused','EMPLOYEE'),
 ('other@example.com','unused','EMPLOYEE'),
 ('hr@example.com','unused','HR'),
 ('admin@example.com','unused','ADMIN'),
 ('noprofile@example.com','unused','EMPLOYEE');
INSERT INTO department(name) VALUES ('Engineering');
INSERT INTO employee("userId","employeeNumber","firstName","lastName","departmentId",position) VALUES
 (1,'EMP001','Rani','Putri',1,'Engineer'),
 (2,'EMP002','Budi',NULL,NULL,NULL),
 (3,'HR001','HR',NULL,NULL,NULL),
 (4,'ADM001','Admin',NULL,NULL,NULL);
