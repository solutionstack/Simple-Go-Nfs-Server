![build](https://github.com/github/docs/actions/workflows/main.yml/badge.svg)

# Network File System in Go

## Overview

This project demonstrates a simple network file system implemented in Go. It explores low-level networking concepts and provides a practical example of how to build a distributed file system. The system operates in two modes: **Master** and **Replica**.

## Features

- **Master and Replica Modes**: The program can be run in either master or replica mode, allowing for flexible configurations based on your needs.
- **Replica Requirement**: At least one replica must be running before starting the master node. This ensures that the master can effectively manage and coordinate file operations across the replicas.         
- **Low-Level Networking Exploration**: The primary goal of this project is to delve into low-level networking concepts in Go, providing insights into how network file systems operate.

## Getting Started

### Prerequisites

- Go (version 1.19 or higher)
- Basic understanding of Go programming and networking concepts

### Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/yourusername/network-file-system.git
   cd network-file-system
   
2. Run one or more replica instances providing `--port` and `--storage-root` cli arguments
   ````bash
   go run main.go replica --port=XXXX --storage-root=/path/for/files

3. Run the master instance which starts an http server to interact with the NFS
    ````bash
   go run main.go master --peers=comma-seperated-list-of-ip-ports --storage-root=/path/for/files --http-port=XXXX

### Usage

#### The http-server exposes two endpoints
    - POST / Which is the main file upload endpoint and expects multipart data,
             data is replicated to all available replicas instances

    - GET /byname?filename= This endpoint expects a previously uplocaded file and a randomly selected replica is used to 
            stream the (if available) to the client

### Demo
1. Create to replicas and one master instance and post a file to the master

[start_servers.webm](https://github.com/user-attachments/assets/81a9198b-52be-4933-86b3-0bb1e807003b)

2. Call a GET request to read the uploaded file which is streamed from a random replica instance
   
[read_file.webm](https://github.com/user-attachments/assets/8a4d24e4-0aae-4833-97ed-c3047bd26a6e)


## Acknowledgments

 ### [Go Programming Language](https://golang.org)
 ### Inspiration from various network file system implementations


