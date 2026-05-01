"""Text analysis example — pure stdlib, no pip dependencies, fast deploy.

Two endpoints are registered:

  POST /analyze  — stateless function, returns word/sentence stats
  POST /readability — stateful class (caches nothing today but shows @enter hook)
"""

import skyscale

app = skyscale.App("text-analyzer")


# ── 1. Stateless function endpoint ──────────────────────────────────────────

@app.function()
@skyscale.web_endpoint(method="POST", path="/analyze")
def analyze(text: str) -> dict:
    """Return word frequency stats for the given text."""
    from collections import Counter
    import re

    if not text or not text.strip():
        return {"error": "text field is required"}

    words = re.findall(r"\b\w+\b", text.lower())
    sentences = [s.strip() for s in re.split(r"[.!?]+", text) if s.strip()]
    paragraphs = [p.strip() for p in text.split("\n\n") if p.strip()]

    if not words:
        return {"error": "no words found in text"}

    word_freq = Counter(words)
    top_words = [{"word": w, "count": c} for w, c in word_freq.most_common(10)]

    avg_word_len = round(sum(len(w) for w in words) / len(words), 2)
    avg_sentence_len = round(len(words) / len(sentences), 1) if sentences else 0
    # Flesch-Kincaid reading ease (approximation without syllable count)
    reading_time_sec = round(len(words) / 3.5)  # ~210 wpm = 3.5 words/sec

    return {
        "word_count": len(words),
        "unique_words": len(set(words)),
        "sentence_count": len(sentences),
        "paragraph_count": len(paragraphs),
        "avg_word_length": avg_word_len,
        "avg_sentence_length_words": avg_sentence_len,
        "estimated_reading_time_seconds": reading_time_sec,
        "top_10_words": top_words,
        "lexical_diversity": round(len(set(words)) / len(words), 3),
    }


# ── 2. Stateful class endpoint ───────────────────────────────────────────────

@app.cls()
class ReadabilityScorer:
    """Scores text readability and keeps a call counter across requests."""

    @skyscale.enter()
    def setup(self):
        self.calls = 0
        # Pre-compile pattern once on container boot
        import re
        self._word_re = re.compile(r"\b\w+\b")
        self._sent_re = re.compile(r"[.!?]+")

    @skyscale.web_endpoint(method="POST", path="/readability")
    def score(self, text: str) -> dict:
        """
        Returns a readability breakdown plus a running call counter
        to prove state persists between requests.
        """
        if not text or not text.strip():
            return {"error": "text field is required"}

        self.calls += 1

        words = self._word_re.findall(text.lower())
        sentences = [s.strip() for s in self._sent_re.split(text) if s.strip()]

        if not words:
            return {"error": "no words found"}

        word_count = len(words)
        sent_count = max(len(sentences), 1)

        # Approximate syllable count (vowel-group heuristic)
        def syllables(word):
            import re
            word = word.lower()
            count = len(re.findall(r"[aeiouy]+", word))
            if word.endswith("e") and count > 1:
                count -= 1
            return max(count, 1)

        total_syllables = sum(syllables(w) for w in words)

        # Flesch Reading Ease: 206.835 - 1.015*(words/sents) - 84.6*(syllables/words)
        fre = round(
            206.835
            - 1.015 * (word_count / sent_count)
            - 84.6 * (total_syllables / word_count),
            1,
        )
        fre = max(0.0, min(100.0, fre))

        if fre >= 90:
            grade = "Very Easy (5th grade)"
        elif fre >= 80:
            grade = "Easy (6th grade)"
        elif fre >= 70:
            grade = "Fairly Easy (7th grade)"
        elif fre >= 60:
            grade = "Standard (8th–9th grade)"
        elif fre >= 50:
            grade = "Fairly Difficult (10th–12th grade)"
        elif fre >= 30:
            grade = "Difficult (College)"
        else:
            grade = "Very Confusing (Professional)"

        return {
            "flesch_reading_ease": fre,
            "grade_level": grade,
            "word_count": word_count,
            "sentence_count": sent_count,
            "syllable_count": total_syllables,
            "avg_syllables_per_word": round(total_syllables / word_count, 2),
            "container_call_count": self.calls,
        }
