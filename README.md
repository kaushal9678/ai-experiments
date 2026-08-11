# 🧪 AI Engineering Experiments

Welcome to my AI Experiments repository! This repository serves as a personal sandbox and portfolio for building advanced, privacy-first Artificial Intelligence tools. 

My core focus here is **Local AI Orchestration**—proving that you can build enterprise-grade, highly autonomous AI agents that run 100% locally on consumer hardware without relying on expensive, privacy-invasive cloud APIs like OpenAI or Anthropic.

## 📁 Projects in this Repository

### [1. Local PDF Q&A with RAG (Python)](./01-local-pdf-rag)
A fully local Retrieval-Augmented Generation (RAG) pipeline built with **Python**, **LangChain**, and **Ollama**. 
- **What it does:** Allows you to chat securely with your PDF documents, extracting facts via semantic search without sending private data to the cloud.
- **Tech Stack:** Python, LangChain, ChromaDB, Qwen3, Nomic-Embed-Text.
- **Key Highlight:** Includes a custom LangChain router agent that dynamically decides whether to use the RAG pipeline or programmatic tools (like fetching the current time).

### [2. Autonomous ADO Code Review Agent (Go)](./02-code-review-agent)
An advanced, multi-stage AI Code Review Agent built from scratch in **Go**. 
- **What it does:** Automatically reviews Azure DevOps Pull Requests by chunking the entire repository into a vector space, retrieving architectural context, and aggressively verifying code logic against ADO/Jira business tickets.
- **Tech Stack:** Go, Azure DevOps REST API, ChromaDB, Ollama (Qwen2.5-Coder).
- **Key Highlight:** Bypasses generic PR summaries by utilizing complex ADO `threadContext` APIs to post actionable AI feedback as nested threads exactly on the modified lines of code in the PR diff.

## 🛠️ Core Technologies Used
Across these experiments, I heavily utilize:
- **Ollama**: For managing and inferencing local LLMs (e.g. `qwen2.5-coder:14b`, `qwen3:8b`, `nomic-embed-text`).
- **ChromaDB**: As the local vector store for fast, high-dimensional semantic search.
- **Languages**: A mix of **Python** (for fast AI prototyping) and **Go** (for blazing fast concurrent disk I/O, strict struct typing, and frictionless binary distribution).

---
*Feel free to explore the individual folders for detailed documentation, architecture diagrams, and installation instructions for each project!*
