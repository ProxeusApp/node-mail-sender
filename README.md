# Proxeus External Node: node-mail-sender
An implementation of an external node email sender as an example Proxeus event module for notification
## Specification

Node Name: Email Sender  

Node Id: node-mail-sender  
Docker Image: proxeus/node-mail-sender:latest  

Implementation: External Node  
Type: Event Module

## Usage

The Email Sender event module enables workflows to send out notifications to users via email.

## Implementation

Sends Events in the form of an email using the Sparkpost service.

## Deployment

The node is available as docker image and can be used within a typical Proxeus Platform setup by including the following docker-compose service:

```
node-mail-sender:
    image: proxeus/node-mail-sender
    container_name: xes_node-mail-sender
    networks:
      - xes-platform-network
    restart: unless-stopped
    environment:
      PROXEUS_INSTANCE_URL: http://xes-platform:1323
      SERVICE_SECRET: "${PROXEUS_SERVICE_SECRET}"
      SERVICE_PORT: 8013
      REGISTER_RETRY_INTERVAL: 3
      SERVICE_URL: http://node-mail-sender:8013
      TZ: Europe/Zurich
      PROXEUS_SPARKPOST_API_KEY: "${PROXEUS_SPARKPOST_API_KEY}"
    ports:
      - "8013:8013"
```

## Configuration

The node needs to be configured with a valid Sparkpost API Key through its ```PROXEUS_SPARKPOST_API_KEY``` variable.
