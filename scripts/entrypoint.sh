#!/bin/sh

# The binaries read their configuration straight from the environment, so there
# is no longer a .env file to fabricate here.

# Run the migrations if the --migrate flag is passed
# Else start the application
if [ "$1" = "--migrate" ]; then
    exec /app/overlord-migrate
else
    exec /app/overlord
fi
