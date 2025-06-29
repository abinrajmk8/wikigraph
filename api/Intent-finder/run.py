from transformers import DistilBertTokenizer, DistilBertForSequenceClassification
import torch

print("start")

model_path = "intent_model"
model = DistilBertForSequenceClassification.from_pretrained(model_path)
tokenizer = DistilBertTokenizer.from_pretrained(model_path)
model.eval()


label_map = {
    0: "Affiliation",
    1: "AwardYear",
    2: "Awarded",
    3: "BirthDate",
    4: "BirthPlace",
    5: "CauseOfDeath",
    6: "Children",
    7: "Citizenship",
    8: "CoFounded",
    9: "CollaboratedWith",
    10: "DeathDate",
    11: "DeathPlace",
    12: "Discovered",
    13: "Employer",
    14: "Field",
    15: "Founded",
    16: "Influenced",
    17: "Invention",
    18: "KnownFor",
    19: "Membership",
    20: "Mentor",
    21: "Nationality",
    22: "PublishedWork",
    23: "Religion",
    24: "Residence",
    25: "Spouse",
    26: "Student",
    27: "StudiedAt",
    28: "Theory",
    29: "WorkedAt"
}

# Inference loop
print(" Intent Classifier Ready (type 'exit' to quit)")
while True:
    query = input("Q.> Your question: ")
    if query.lower() in ["exit", "quit"]:
        break

    inputs = tokenizer(query, return_tensors="pt", truncation=True, padding=True)
    with torch.no_grad():
        outputs = model(**inputs)
        predicted_class = outputs.logits.argmax().item()
        intent = label_map[predicted_class]

    print(f" Predicted intent: {intent}\n")
