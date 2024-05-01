#!/bin/sh

# Add environment variables to /app/.env

echo "PORT=$OVERLORD_PORT" > /app/.env
echo "DB_URL=$OVERLORD_DB_URL" >> /app/.env

# Run the migrations if the --migrate flag is passed
if [ "$1" = "--migrate" ]; then
    /app/overlord-migrate
fi

# Start the app
/app/overlord