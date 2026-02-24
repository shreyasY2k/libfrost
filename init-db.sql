-- Initialize databases for the enterprise LLM stack
-- PostgreSQL serves both Keycloak and Bifrost with separate databases

-- Create Keycloak database and user
CREATE DATABASE keycloak;
CREATE USER keycloak_user WITH ENCRYPTED PASSWORD 'keycloak_password';
GRANT ALL PRIVILEGES ON DATABASE keycloak TO keycloak_user;
ALTER DATABASE keycloak OWNER TO keycloak_user;

-- Create Bifrost database and user
CREATE DATABASE bifrost;
CREATE USER bifrost_user WITH ENCRYPTED PASSWORD 'bifrost_password';
GRANT ALL PRIVILEGES ON DATABASE bifrost TO bifrost_user;
ALTER DATABASE bifrost OWNER TO bifrost_user;

-- Grant schema access (required for PostgreSQL 15+)
\c keycloak
GRANT ALL ON SCHEMA public TO keycloak_user;

\c bifrost
GRANT ALL ON SCHEMA public TO bifrost_user;
