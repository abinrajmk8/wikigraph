import React, { useState } from "react";

function App() {
  const [question, setQuestion] = useState("");
  const [answer, setAnswer] = useState("");

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!question.trim()) return;

    try {
      const res = await fetch(`http://localhost:8080/query?q=${encodeURIComponent(question)}`);
      const data = await res.json();
      setAnswer(JSON.stringify(data, null, 2));
    } catch (err) {
      setAnswer("Error fetching data");
    }
  };

  return (
    <div style={styles.page}>
      {/* HEADER */}
      <header style={styles.header}>
        <h1 style={styles.logo}>
          Wiki<span style={{ color: "red" }}>G</span>raph
        </h1>
        <nav style={styles.nav}>
          <button style={{ ...styles.navItem, ...styles.activeNav }}>Home</button>
          <button style={styles.navItem}>about</button>
          <button style={styles.navItem}>contact</button>
          <button style={styles.navItem}>visualize</button>
        </nav>
      </header>

      {/* MAIN */}
      <main style={styles.main}>
        <h2 style={styles.askText}>Ask me...</h2>

        <form onSubmit={handleSubmit} style={styles.form}>
          <input
            type="text"
            placeholder="eg .When was Albert Einstein born?"
            value={question}
            onChange={(e) => setQuestion(e.target.value)}
            style={styles.input}
          />
          <button type="submit" style={styles.sendButton}>
            <svg
              width="30"
              height="30"
              fill="none"
              stroke="#00ffff"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
              viewBox="0 0 24 24"
            >
              <path d="M22 2L11 13"></path>
              <path d="M22 2L15 22L11 13L2 9L22 2Z"></path>
            </svg>
          </button>
        </form>

        {/* ANSWER */}
        {answer && (
          <div style={styles.answerBox}>
            <pre style={styles.answerText}>{answer}</pre>
          </div>
        )}
      </main>
    </div>
  );
}

const styles = {
  page: {
    background: "#000 radial-gradient(circle at center, #111, #000)",
    color: "#fff",
    minHeight: "100vh",
    padding: "40px 20px",
    fontFamily: "Segoe UI, sans-serif",
  },
  header: {
    display: "flex",
    justifyContent: "space-between",
    alignItems: "center",
    marginBottom: "60px",
  },
  logo: {
    fontSize: "32px",
    fontWeight: "bold",
  },
  nav: {
    backgroundColor: "#000",
    border: "2px solid #00ffff",
    borderRadius: "30px",
    padding: "10px 20px",
    display: "flex",
    gap: "25px",
    boxShadow: "0 0 10px #00ffff",
  },
  navItem: {
    background: "none",
    border: "none",
    color: "#fff",
    fontSize: "16px",
    cursor: "pointer",
    textTransform: "capitalize",
  },
  activeNav: {
    fontWeight: "bold",
    textDecoration: "underline",
  },
  main: {
    maxWidth: "600px",
    margin: "0 auto",
    textAlign: "center",
  },
  askText: {
    fontSize: "20px",
    color: "#ccc",
    marginBottom: "20px",
  },
  form: {
    display: "flex",
    alignItems: "center",
    gap: "10px",
    marginBottom: "30px",
  },
  input: {
    flex: 1,
    padding: "12px 16px",
    fontSize: "16px",
    borderRadius: "12px",
    backgroundColor: "#111",
    border: "2px solid #00ffff",
    color: "#fff",
    outline: "none",
  },
  sendButton: {
    background: "none",
    border: "none",
    cursor: "pointer",
  },
  answerBox: {
    backgroundColor: "#111",
    padding: "20px",
    borderRadius: "15px",
    border: "1px solid #00ffff",
    minHeight: "150px",
    color: "#0ff",
    textAlign: "left",
    whiteSpace: "pre-wrap",
  },
  answerText: {
    fontSize: "16px",
    color: "#0ff",
  },
};

export default App;
