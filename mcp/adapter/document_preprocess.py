from pathlib import Path
from dataclasses import dataclass
from typing import Dict, Any, List

from langchain_text_splitters import RecursiveCharacterTextSplitter, Language
from pypdf import PdfReader

from logging import getLogger
logger = getLogger(__name__)

SUPPORTED_LANG = [e.value for e in Language]
EXT_PATTERN = {
    "PYTHON": ["*.py"],
    "DOCUMENT": ["*.md", "*.mdx"],
}
CHUNK_OPTS = {
    "PYTHON": {"chunk_size": 800, "chunk_overlap": 200},
    "DOCUMENT": {"chunk_size": 1000, "chunk_overlap": 200},
}

@dataclass
class DocumentFile:
    path: str
    content: str


class DocumentPreprocessingAdapter:
    """
    Adapter for document preprocessing using LangChain.
    """
    def __init__(self) -> None:
        pass

    def preprocess_document_from_text(
        self,
        texts: List[str],
    ) -> None:
        """
        Preprocess documents from the given text inputs
        """
        # check language support
        language = self._check_language_supported(language="DOCUMENT")
        # Load documents from the given files path
        documents = self._load_documents_from_text(texts)
        # get text splitter
        splitter = self._get_splitter(language)
        # Chunk documents
        chunks = self._chunk_documents(documents, splitter)
        return chunks

    def preprocess_documents_from_path(
        self,
        path: str,
        language: str = None,
    ) -> None:
        """
        Preprocess documents from the given path
        """
        # check language support
        language = self._check_language_supported(language)
        # Load documents from the given files path
        documents = self._load_documents_from_path(path, language)
        # get text splitter
        splitter = self._get_splitter(language)
        # Chunk documents
        chunks = self._chunk_documents(documents, splitter)
        return chunks

    def _check_language_supported(self, language: str) -> bool:
        if language is None:
            language = "DOCUMENT"
        language = language.upper()
        if language not in EXT_PATTERN.keys():
            raise ValueError(f"Unsupported language for document preprocessing: {language}")
        return language

    def _load_documents_from_text(self, texts: List[str]) -> List[DocumentFile]:
        doc_files = [
            DocumentFile(path=f"input_text_{idx}", content=text)
            for idx, text in enumerate(texts)
        ]
        logger.info(f"{len(doc_files)} text document loaded")
        return doc_files

    def _load_documents_from_path(self, path: str, language: str = None) -> List[DocumentFile]:
        root = Path(path)
        doc_files: List[DocumentFile] = []

        patterns = EXT_PATTERN[language]

        if root.endswith(".pdf"):
            reader = PdfReader(str(root))
            doc_files = []

            for i, page in enumerate(reader.pages):
                try:
                    text = page.extract_text() or ""
                except Exception as e:
                    text = f"[Error reading page {i}: {e}]"
                doc_files.append(DocumentFile(path=f"{root.name}::page-{i}", content=text))

        else:
            for pattern in patterns:
                for path in root.glob(pattern):
                    if any(part.startswith(".") for part in path.parts):
                        continue

                    try:
                        text = path.read_text(encoding="utf-8")
                    except UnicodeDecodeError:
                        text = path.read_text(encoding="utf-8", errors="ignore")

                    rel_path = str(path.relative_to(root))
                    doc_files.append(DocumentFile(path=rel_path, content=text))

        logger.info(f"{len(doc_files)} python files loaded")

        return doc_files

    def _get_splitter(
        self,
        language: str = None,
    ) -> RecursiveCharacterTextSplitter:
        """
        Get text splitter based on language
        """
        chunk_kwargs = CHUNK_OPTS[language]
        if language == "DOCUMENT":
            return RecursiveCharacterTextSplitter(
                **chunk_kwargs
            )

        splitter = RecursiveCharacterTextSplitter.from_language(
            language=getattr(Language, language),
            **chunk_kwargs
        )

        return splitter

    def _chunk_documents(
        self,
        document_files: List[DocumentFile],
        splitter: RecursiveCharacterTextSplitter,
    ) -> List[Dict[str, Any]]:
        """
        Create chunks from Document of Python code files
        """
        chunks: List[Dict[str, Any]] = []

        for code_file in document_files:
            split_texts = splitter.split_text(code_file.content)

            for idx, chunk_text in enumerate(split_texts):
                chunk = {
                    "id": f"{code_file.path}::chunk-{idx}",
                    "text": chunk_text,
                    "metadata": {
                        "source": code_file.path,
                        "chunk_index": idx,
                    },
                }
                chunks.append(chunk)

        logger.info(f"{len(chunks)} chunks created from documents")

        return chunks
