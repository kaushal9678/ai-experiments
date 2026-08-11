import os

try:
    from dotenv import load_dotenv
except ImportError:
    def load_dotenv(env_path=".env"):
        if not os.path.exists(env_path):
            print("Warning: python-dotenv is not installed and .env was not found. Using existing environment variables.")
            return False

        print("Warning: python-dotenv is not installed. Loading .env manually.")
        with open(env_path, "r", encoding="utf-8") as env_file:
            for line in env_file:
                line = line.strip()
                if not line or line.startswith("#") or "=" not in line:
                    continue
                key, value = line.split("=", 1)
                key = key.strip()
                value = value.strip().strip('"').strip("'")
                if key and key not in os.environ:
                    os.environ[key] = value
        return True

from langchain_community.document_loaders import DirectoryLoader, UnstructuredFileLoader
from langchain_text_splitters import RecursiveCharacterTextSplitter
from langchain_ollama import OllamaEmbeddings
from langchain_community.embeddings import HuggingFaceEmbeddings
try:
    from langchain_chroma import Chroma
except ImportError:
    from langchain_community.vectorstores import Chroma
    print("Warning: langchain_chroma is not installed. Falling back to langchain_community.vectorstores.Chroma.")
from langchain_ollama import ChatOllama
from langchain_core.prompts import ChatPromptTemplate
from langchain_core.runnables import RunnablePassthrough
from langchain_core.output_parsers import StrOutputParser

BASE_DIR = os.path.dirname(os.path.abspath(__file__))
CHROMA_PATH = os.path.join(BASE_DIR, "chroma_db") # Directory to store ChromaDB data

load_dotenv()
# Step 1: Prepare Your Data
DATA_DIR = os.getenv("DATA_DIR", BASE_DIR)
if not os.path.isabs(DATA_DIR):
    DATA_DIR = os.path.normpath(os.path.join(BASE_DIR, DATA_DIR))
PDF_FILE = os.getenv("PDF_FILE", "data-intensive-application.pdf")


# Step 2: Load Documents in Python
def load_documents():
    """Loads all documents from the specified data directory."""
    print(f"Scanning directory: {DATA_DIR} for documents...")
    
    # DirectoryLoader with UnstructuredFileLoader will automatically handle PDFs, TXT, MD, DOCX, etc.
    loader = DirectoryLoader(
        DATA_DIR,
        glob="**/*.*", # Load all file types
        loader_cls=UnstructuredFileLoader,
        show_progress=True
    )
    documents = loader.load()
    print(f"Loaded {len(documents)} document section(s) from directory {DATA_DIR}")
    return documents

# documents = load_documents() # Call this later

# Step 3: Split Documents
def split_documents(documents):
    """Splits documents into smaller chunks."""
    text_splitter = RecursiveCharacterTextSplitter(
        chunk_size=1000,
        chunk_overlap=200,
        length_function=len,
        is_separator_regex=False,
    )
    all_splits = text_splitter.split_documents(documents)
    print(f"Split into {len(all_splits)} chunks")
    return all_splits

# loaded_docs = load_documents()
# chunks = split_documents(loaded_docs) # Call this later

# Step 4: Choose and Configure Embedding Model
# Option A (Recommended for Simplicity): Ollama Embeddings
def get_embedding_function(model_name="nomic-embed-text"):
    """Initializes the Ollama embedding function."""
    # Ensure Ollama server is running (ollama serve)
    embeddings = OllamaEmbeddings(model=model_name)
    print(f"Initialized Ollama embeddings with model: {model_name}")
    return embeddings

# embedding_function = get_embedding_function() # Call this later

# Step 5: Set Up Local Vector Store (ChromaDB)
def get_vector_store(embedding_function, persist_directory=CHROMA_PATH):
    """Initializes or loads the Chroma vector store."""
    vectorstore = Chroma(
        persist_directory=persist_directory,
        embedding_function=embedding_function
    )
    print(f"Vector store initialized/loaded from: {persist_directory}")
    return vectorstore

# Step 6: Index Documents (Embed and Store)
def index_documents(chunks, embedding_function, persist_directory=CHROMA_PATH):
    """Indexes document chunks into the Chroma vector store."""
    print(f"Indexing {len(chunks)} chunks...")
    vectorstore = Chroma.from_documents(
        documents=chunks,
        embedding=embedding_function,
        persist_directory=persist_directory
    )
    if hasattr(vectorstore, "persist"):
        vectorstore.persist()
        print(f"Indexing complete. Data saved to: {persist_directory}")
    else:
        print(f"Indexing complete. Persist not required for this Chroma implementation.")
    return vectorstore

# --- Removed premature execution from module import-time ---
# Step 7: Build the RAG Chain
def create_rag_chain(vector_store, llm_model_name="qwen3:8b", context_window=8192):
    """Creates the RAG chain."""
    # Initialize the LLM where context_window is set to the desired size (e.g., 8192 tokens)
    llm = ChatOllama(
        model=llm_model_name,
        temperature=0, # Lower temperature for more factual RAG answers
        num_ctx=context_window # IMPORTANT: Set context window size
    )
    print(f"Initialized ChatOllama with model: {llm_model_name}, context window: {context_window}")

    # Create the retriever
    retriever = vector_store.as_retriever(
        search_type="similarity", # Or "mmr"
        search_kwargs={'k': 3} # Retrieve top 3 relevant chunks
    )
    print("Retriever initialized.")

    # Define the prompt template
    template = """Answer the question based ONLY on the following context:
{context}

Question: {question}
"""
    prompt = ChatPromptTemplate.from_template(template)
    print("Prompt template created.")

    # Define the RAG chain using LCEL
    rag_chain = (
        {"context": retriever, "question": RunnablePassthrough()}
| prompt
| llm
| StrOutputParser()
    )
    print("RAG chain created.")
    return rag_chain

# Step 8: Query the RAG Chain
def query_rag_chain(rag_chain, question):
    """Queries the RAG chain with a user question."""
    print(f"Querying RAG chain with question: {question}")
    answer = rag_chain.invoke({"question": question})
    print(f"Answer: {answer}")
    return answer


def query_pdf(question, reindex=False, llm_model_name="qwen3:8b", context_window=8192):
    """Queries the configured PDF using the local RAG setup."""
    print(f"query_pdf: {question}")
    docs = load_documents()
    chunks = split_documents(docs)
    embedding_function = get_embedding_function()

    if reindex or not os.path.isdir(CHROMA_PATH) or not os.listdir(CHROMA_PATH):
        vector_store = index_documents(chunks, embedding_function)
    else:
        vector_store = get_vector_store(embedding_function)

    retriever = vector_store.as_retriever(
        search_type="similarity",
        search_kwargs={"k": 3}
    )

    if callable(retriever):
        retrieved_docs = retriever(question)
    elif hasattr(retriever, "invoke"):
        retrieved_docs = retriever.invoke(question)
    elif hasattr(retriever, "get_relevant_documents"):
        retrieved_docs = retriever.get_relevant_documents(question)
    else:
        raise RuntimeError("Unable to call retriever for question lookup")

    if hasattr(retrieved_docs, "__iter__") and not isinstance(retrieved_docs, (str, bytes)):
        context = "\n\n".join(
            getattr(doc, "page_content", str(doc)) for doc in retrieved_docs
        )
    else:
        context = str(retrieved_docs)

    prompt = ChatPromptTemplate.from_template(
        """Answer the question based ONLY on the following context:
{context}

Question: {question}
"""
    )
    prompt_value = prompt.format_prompt(context=context, question=question)

    llm = ChatOllama(
        model=llm_model_name,
        temperature=0,
        num_ctx=context_window,
    )
    answer = llm.invoke(prompt_value)

    if hasattr(answer, "content"):
        return answer.content
    if isinstance(answer, dict) and "messages" in answer:
        return "\n".join(getattr(msg, "content", "") for msg in answer["messages"] if getattr(msg, "content", None))
    return str(answer)

# --- Main Execution ---
if __name__ == "__main__":
    # 1. Load Documents
    docs = load_documents()

    # 2. Split Documents
    chunks = split_documents(docs)

    # 3. Get Embedding Function
    embedding_function = get_embedding_function() # Using Ollama nomic-embed-text

    # 4. Index Documents (Only needs to be done once per document set)
    # Check if DB exists, if not, index. For simplicity, we might re-index here.
    # A more robust approach would check if indexing is needed.
    print("Attempting to index documents...")
    vector_store = index_documents(chunks, embedding_function)
    # To load existing DB instead:
    # vector_store = get_vector_store(embedding_function)

    # 5. Create RAG Chain
    rag_chain = create_rag_chain(vector_store, llm_model_name="qwen3:8b") # Use the chosen Qwen 3 model

    # 6. Query
    query_question = "What is the main topic of the document?" # Replace with a specific question
    query_rag_chain(rag_chain, query_question)

    query_question_2 = "Summarize the introduction section." # Another example
    query_rag_chain(rag_chain, query_question_2)