from flask import Flask, request, jsonify
from transformers import AutoTokenizer, AutoModelForSequenceClassification
from huggingface_hub import login
from dotenv import load_dotenv
import os
import torch

# Load .env variables
load_dotenv()
login(os.getenv("HF_TOKEN"))  # Authenticate to Hugging Face

app = Flask(__name__)

# Load model and tokenizer from Hugging Face
model_path = "abinrajmk8/intent-bert"
tokenizer = AutoTokenizer.from_pretrained(model_path)
model = AutoModelForSequenceClassification.from_pretrained(model_path)
model.eval()

# Label map (should match training)
label_map = {
    0: "Affiliation", 1: "AwardYear", 2: "Awarded", 3: "BirthDate", 4: "BirthPlace",
    5: "CauseOfDeath", 6: "Children", 7: "Citizenship", 8: "CoFounded", 9: "CollaboratedWith",
    10: "DeathDate", 11: "DeathPlace", 12: "Discovered", 13: "Employer", 14: "Field",
    15: "Founded", 16: "Influenced", 17: "Invention", 18: "KnownFor", 19: "Membership",
    20: "Mentor", 21: "Nationality", 22: "PublishedWork", 23: "Religion", 24: "Residence",
    25: "Spouse", 26: "Student", 27: "StudiedAt", 28: "Theory", 29: "WorkedAt"
}

@app.route("/predict", methods=["POST"])
def predict():
    data = request.get_json()
    sentence = data.get("text", "")

    if not sentence:
        return jsonify({"error": "No input text provided"}), 400

    inputs = tokenizer(sentence, return_tensors="pt", truncation=True, padding=True)
    with torch.no_grad():
        outputs = model(**inputs)
        predicted_class = outputs.logits.argmax().item()
        intent = label_map[predicted_class]

    return jsonify({"intent": intent})


if __name__ == "__main__":
    port = int(os.environ.get("PORT", 5000))  # fallback to 5000 locally
    app.run(host="0.0.0.0", port=port)

