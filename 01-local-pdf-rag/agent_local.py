from concurrent.futures import Executor
import os
from dotenv import load_dotenv
from langchain.tools import tool
import datetime
from langchain_ollama import ChatOllama
from langchain.agents import create_agent
import rag_local

load_dotenv() # Optional
# Step 1: Define Custom Tools
@tool
def get_current_datetime(format: str = "%Y-%m-%d %H:%M:%S") -> str:
    """
    Returns the current date and time, formatted according to the provided Python strftime format string.
    Example format strings: '%Y-%m-%d' for date, '%H:%M:%S' for time.
    If no format is specified, defaults to '%Y-%m-%d %H:%M:%S'.
    """
    try:
        return datetime.datetime.now().strftime(format)
    except Exception as e:
        return f"Error formatting date/time: {e}"

@tool
def query_pdf(question: str) -> str:
    """Queries the PDF document loaded by rag_local.py."""
    try:
        return rag_local.query_pdf(question)
    except Exception as e:
        return f"Error querying PDF: {e}"

# List of tools the agent can use
tools = [get_current_datetime, query_pdf]
print("Custom tool defined.")


def is_date_time_question(question: str) -> bool:
    lower = question.lower()
    return any(token in lower for token in ["date", "time", "current time", "current date", "today", "now"])


def run_agent(user_input: str):
    """Runs the question through the appropriate local handler."""
    print("\nInvoking agent...")
    print(f"Input: {user_input}")

    if is_date_time_question(user_input):
        response = get_current_datetime.invoke({})
    else:
        response = query_pdf.invoke({"question": user_input})

    print("\nAgent Response:")
    print(response)


# --- Main Execution ---
if __name__ == "__main__":
    while True:
        user_input = input("Ask a question (or press Enter to exit): ").strip()
        if not user_input:
            print("Goodbye.")
            break
        run_agent(user_input)
