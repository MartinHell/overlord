#!/bin/sh

# Add environment variables to /app/.env

echo "PORT=$OVERLORD_PORT" > /app/.env
echo "DB_URL=$OVERLORD_DB_URL" >> /app/.env

# Start the app
/app/overlord