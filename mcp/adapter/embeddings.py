from typing import List, Union

import numpy as np


class EmbeddingAdapter:
    """
    General Adapter for various embedding SDK interactions.
    """
    def __init__(self, mode: str, model_name: str) -> None:
        self.mode = mode
        self.model_name = model_name

        if mode in ["fastembed", "femb"]:
            self.adapter = FastEmbedSDKAdapter(model_name)
        elif mode in ["sbert", "sentence_transformer"]:
            self.adapter = SBERTSDKAdapter(model_name)
        elif mode in ["huggingface", "hf"]:
            self.adapter = HuggingFaceSDKAdapter(model_name)
        elif mode == "openai":
            self.adapter = OpenAISDKAdapter(model_name)
        else:
            raise ValueError(f"Unsupported embedding mode: {mode}")

    def get_embedding(self, texts: List[str]) -> Union[List[float], List[List[float]], np.ndarray]:
        """
        Retrieves embeddings for a list of texts using the specified SDK.

        Args:
            texts (List[str]): A list of texts to embed.

        Returns:
            np.ndarray: List of embeddings where each row corresponds to the embedding of a text
        """
        embeddings = self.adapter.get_embedding(texts)

        # l2 normalize
        embeddings = self._normalize_embeddings(np.array(embeddings))
        assert embeddings.shape[0] == len(texts)
        return embeddings.tolist()

    def _normalize_embeddings(self, embeddings: np.ndarray) -> np.ndarray:
        # l2 normalize and guard against zero vectors
        norm = np.linalg.norm(embeddings, axis=1, keepdims=True)
        epsilon = 1e-12
        norm = np.maximum(norm, epsilon)
        embeddings = embeddings / norm
        return embeddings


class FastEmbedSDKAdapter:
    """
    Adapter for FastEmbed SDK interactions.
    """
    def __init__(self, model_name: str = "fastembed/fastembed-base") -> None:
        """
        Initializes the FastEmbedSDKAdapter with the provided model name.

        Args:
            model_name (str): The name of the FastEmbed model to use.
        """

        from fastembed import TextEmbedding

        self.model = TextEmbedding(model_name)


    def get_embedding(self, texts: List[str]) -> Union[List[float], List[List[float]], np.ndarray]:
        """
        Retrieves the embedding for the given text using FastEmbed SDK.
        """
        embeddings = list(self.model.embed(texts))
        return embeddings


class SBERTSDKAdapter:
    """
    Adapter for SBERT (Sentence Transformer) SDK interactions.
    """
    def __init__(self, model_name: str = "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2") -> None:
        """
        Initializes the SBERTSDKAdapter with the provided model name.

        Args:
            model_name (str): The name of the Sentence Transformer model to use.
        """

        from sentence_transformers import SentenceTransformer

        self.model = SentenceTransformer(model_name, trust_remote_code=True)


    def get_embedding(self, texts: List[str]) -> Union[List[float], List[List[float]], np.ndarray]:
        """
        Retrieves the embedding for the given text using Sentence Transformer SDK.
        """
        return self.model.encode(texts)


class HuggingFaceSDKAdapter(EmbeddingAdapter):
    """
    Adapter for HuggingFace SDK interactions.
    """
    def __init__(self, model_name: str = "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2", cache_dir: str = None) -> None:
        """
        Initializes the HuggingFaceSDKAdapter with the provided model name and cache directory.

        Args:
            model_name (str): The name of the HuggingFace model to use.
            cache_dir (str): The directory to cache the model.
        """

        from transformers import AutoTokenizer, AutoModel

        self.tokenizer = AutoTokenizer.from_pretrained(model_name, cache_dir=cache_dir)
        self.model = AutoModel.from_pretrained(model_name, cache_dir=cache_dir)

    def get_embedding(self, texts: List[str]) -> Union[List[float], List[List[float]], np.ndarray]:
        """
        Retrieves the embedding for the given text using HuggingFace SDK.
        """
        for text in texts:
            # Tokenize sentences
            encoded_input = self.tokenizer(text, padding=True, truncation=True, return_tensors='pt', max_length=512)

        # Compute token embeddings
        embeddings = self.model(**encoded_input).last_hidden_state[:,0,:]

        return embeddings.detach().numpy()


class OpenAISDKAdapter:
    """
    Adapter for OpenAI API interactions.
    """
    def __init__(self, model_name: str) -> None:
        """
        Initializes the OpenAISDKAdapter with the provided model name.

        Args:
            model_name (str): The OpenAI model name.
        """

        import openai

        self.model_name = model_name
        self.client = openai.OpenAI()

    def get_embedding(self, texts: List[str]) -> Union[List[float], List[List[float]], np.ndarray]:
        """
        Retrieves embeddings for a list of texts using OpenAI API.
        """
        response = self.client.embeddings.create(
            input=texts,
            model=self.model_name,
            encoding_format="float",
        )
        outputs = np.array([e.embedding for e in response.data])
        return outputs
